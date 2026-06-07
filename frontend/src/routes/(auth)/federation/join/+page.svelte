<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { t } from '$lib/i18n';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { getApiClient } from '$lib/api/client';
	import { federation as federationApi } from '$lib/api/endpoints/federation';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { ApiError } from '$lib/api/errors';
	import type { JoinInvite, JoinPreview, JoinResult } from '$lib/api/types';
	import {
		buildCrossInstanceRedirect,
		clearPendingInvite,
		loadPendingInvite,
		normalizeInstanceUrl,
		parseInviteHash,
		parseOwnerHash,
		sameInstance,
		stashPendingInvite,
		type ParsedInvite,
		type PendingJoin
	} from '$lib/federation/join';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	// Local alias for the parsed (id, secret) invite the flow operates on.
	type Invite = ParsedInvite;

	const auth = getAuthStore();

	// phase drives what the page renders:
	//   reading      — parsing the invite + resolving auth
	//   invalid      — no usable invite in the link (US-2.1 AC1 negative)
	//   crossInstance — opened on the OWNER's instance: retarget to your own (US-2.1 AC2)
	//   preview      — authed on the joiner: show the project preview + Accept (US-2.1 AC3)
	//   handshake    — accepting: verifying with the owner (US-2.1 AC4)
	//   snapshot     — accepting: copying the project (US-2.1 AC4)
	//   done         — joined; offer to open the project (US-2.1 AC4)
	type Phase = 'reading' | 'invalid' | 'crossInstance' | 'preview' | 'handshake' | 'snapshot' | 'done';

	let phase = $state<Phase>('reading');
	let invite = $state<Invite | null>(null);
	// ownerUrl is the instance that issued the invite. It comes from the `owner`
	// fragment param (carried by a cross-instance redirect) and falls back to the
	// page origin — the link's host IS the owner for a freshly issued link.
	let ownerUrl = $state('');
	let preview = $state<JoinPreview | null>(null);
	let result = $state<JoinResult | null>(null);
	let error = $state<string | null>(null);
	let previewLoading = $state(false);
	// The instance address the visitor types when this is NOT their own instance
	// — "Open in your instance" retargets the invite there (US-2.1 AC2).
	let otherInstance = $state('');

	function currentOrigin(): string {
		return typeof window !== 'undefined' ? window.location.origin : '';
	}

	// resolveJoin pulls the invite + owner from the URL fragment, falling back to a
	// previously stashed join context (the post-login resume path, US-2.1 AC5).
	// When the fragment carries no explicit owner the link's host is the owner, so
	// the page origin is used.
	function resolveJoin(): PendingJoin | null {
		const hash = typeof window !== 'undefined' ? window.location.hash : '';
		const fromHash = parseInviteHash(hash);
		if (fromHash) {
			return { invite: fromHash, owner: parseOwnerHash(hash) ?? currentOrigin() };
		}
		return loadPendingInvite();
	}

	function joinBody(inv: Invite): JoinInvite {
		return { inviteId: inv.inviteId, secret: inv.secret, ownerInstanceUrl: ownerUrl };
	}

	async function loadPreview(inv: Invite): Promise<void> {
		previewLoading = true;
		error = null;
		try {
			// Server-side preview: the secret rides in the request body to OUR
			// instance, which fetches the owner — never browser→owner (US-2.1 AC3).
			preview = await federationApi.preview(getApiClient(), joinBody(inv));
			phase = 'preview';
		} catch (err) {
			error = describeJoinError(err, $t('federation.join.previewFailed'));
			phase = 'preview';
		} finally {
			previewLoading = false;
		}
	}

	async function onAccept(): Promise<void> {
		if (!invite) return;
		error = null;
		phase = 'handshake';
		try {
			// The owner handshake then the buffer-first snapshot bootstrap run
			// server-side; the stepper reflects both stages (US-2.1 AC4 / US-2.3). The
			// single join call drives handshake + snapshot apply; on success the local
			// federated project exists, so the projects store is refreshed to surface
			// it (US-2.3). A mid-stream failure rolls the whole bootstrap back and
			// returns to the preview so the user can retry (US-2.3 AC5).
			phase = 'snapshot';
			result = await federationApi.join(getApiClient(), joinBody(invite));
			clearPendingInvite();
			// Pull the newly bootstrapped federated project into the store so it
			// appears in the UI without a manual reload (US-2.3).
			await projectsStore.load();
			phase = 'done';
		} catch (err) {
			error = describeJoinError(err, $t('federation.join.snapshotFailed'));
			phase = 'preview';
		}
	}

	// describeJoinError maps the typed federation errors the owner can return
	// through the join transport (Federation v1 F2.2): a no-version 400, a generic
	// wrong-secret/unknown-invite 401, a key-mismatch 409, and a stale-invite 410
	// each get a distinct, actionable message (US-2.2 AC4/AC5, US-9.1 AC2).
	function describeJoinError(err: unknown, fallback: string): string {
		if (err instanceof ApiError) {
			switch (err.code) {
				case 'federation_version_unsupported':
					return $t('federation.join.versionUnsupported');
				case 'federation_signature_invalid':
					return $t('federation.join.errors.inviteInvalid');
				case 'federation_key_mismatch':
					return $t('federation.join.errors.keyMismatch');
				case 'federation_untrusted':
					return $t('federation.join.errors.untrusted');
				case 'gone':
					return $t('federation.join.errors.inviteGone');
				default:
					return err.message || fallback;
			}
		}
		return err instanceof Error ? err.message : fallback;
	}

	function openProject(): void {
		if (!result) return;
		void goto(resolve('/(app)/project/[id]', { id: String(result.projectId) }));
	}

	// openInYourInstance retargets the invite to the visitor's own instance,
	// carrying the secret AND the owner URL in the fragment so neither reaches a
	// server as a query parameter and the joiner instance knows which owner to
	// handshake (US-2.1 AC2). Requires a parsed invite to forward.
	function openInYourInstance(): void {
		if (!invite) return;
		const origin = normalizeInstanceUrl(otherInstance);
		if (!origin) return;
		window.location.href = buildCrossInstanceRedirect(origin, invite, ownerUrl);
	}

	// Drive the flow once auth has settled. When the invite's owner IS this origin
	// the page is being served BY the owner — you cannot join from here, so direct
	// the visitor to their own instance (US-2.1 AC2). Otherwise this is the joiner:
	// unauthenticated visitors stash the join context and are sent to login so the
	// flow resumes afterwards (US-2.1 AC5); authenticated ones load the preview
	// against the resolved owner (US-2.1 AC3).
	$effect(() => {
		if (phase !== 'reading') return;
		if (auth.status === 'loading') return;

		const ctx = resolveJoin();
		if (!ctx) {
			phase = 'invalid';
			return;
		}
		invite = ctx.invite;
		ownerUrl = ctx.owner;

		if (sameInstance(ctx.owner, currentOrigin())) {
			phase = 'crossInstance';
			return;
		}

		if (auth.status !== 'authenticated') {
			stashPendingInvite(ctx.invite, ctx.owner);
			void goto(resolve('/login'));
			return;
		}

		void loadPreview(ctx.invite);
	});

	const ownerIdentity = $derived(
		preview ? `${preview.ownerDisplayName} @ ${preview.ownerInstanceUrl}` : ''
	);
	const accepting = $derived(phase === 'handshake' || phase === 'snapshot');
</script>

{#snippet crossInstance()}
	<div class="flex flex-col gap-1.5 rounded-md border border-border p-4">
		<p class="text-sm font-medium text-foreground">
			{$t('federation.join.crossInstanceTitle')}
		</p>
		{#if ownerUrl}
			<p class="text-xs text-muted-foreground">
				{$t('federation.join.crossInstanceFrom', { values: { owner: ownerUrl } })}
			</p>
		{/if}
		<p class="text-xs text-muted-foreground">{$t('federation.join.crossInstancePrompt')}</p>
		<form
			class="mt-1 flex items-center gap-2"
			onsubmit={(e) => {
				e.preventDefault();
				openInYourInstance();
			}}
		>
			<Label class="sr-only" for="other-instance">
				{$t('federation.join.crossInstanceTitle')}
			</Label>
			<Input
				id="other-instance"
				bind:value={otherInstance}
				placeholder={$t('federation.join.instancePlaceholder')}
				class="flex-1"
			/>
			<Button type="submit" variant="outline" size="sm" disabled={!otherInstance.trim()}>
				{$t('federation.join.openInYourInstance')}
			</Button>
		</form>
	</div>
{/snippet}

<div class="flex flex-col gap-4">
	<h1 class="text-lg font-semibold">{$t('federation.join.title')}</h1>

	{#if phase === 'reading'}
		<p class="text-sm text-muted-foreground">{$t('federation.join.loading')}</p>
	{:else if phase === 'invalid'}
		<p class="text-sm text-destructive">{$t('federation.join.invalidInvite')}</p>
	{:else if phase === 'crossInstance'}
		{@render crossInstance()}
	{:else if phase === 'done'}
		<p class="text-sm text-foreground">
			{$t('federation.join.steps.done', { values: { name: result?.projectName ?? '' } })}
		</p>
		<Button type="button" onclick={openProject}>{$t('federation.join.openProject')}</Button>
	{:else}
		{#if previewLoading}
			<p class="text-sm text-muted-foreground">{$t('federation.join.previewLoading')}</p>
		{/if}

		{#if preview}
			<div class="flex flex-col gap-1.5 rounded-md border border-border bg-muted/30 p-4">
				<p class="text-xs text-muted-foreground">{$t('federation.join.preview.heading')}</p>
				<p class="text-base font-medium text-foreground">{preview.projectName}</p>
				<p class="text-xs text-muted-foreground">
					{$t('federation.join.preview.ownedBy', { values: { owner: ownerIdentity } })}
				</p>
				<p class="text-xs text-muted-foreground">
					{$t('federation.join.preview.permissions')}:
					{$t(`federation.join.preview.permission.${preview.permissions}`)}
				</p>
			</div>
		{/if}

		{#if accepting}
			<p class="text-sm text-muted-foreground">
				{phase === 'handshake'
					? $t('federation.join.steps.handshake')
					: $t('federation.join.steps.snapshot')}
			</p>
		{/if}

		{#if error}
			<p class="text-xs text-destructive">{error}</p>
		{/if}

		{#if preview}
			<div class="flex gap-2">
				<Button type="button" disabled={accepting} onclick={onAccept}>
					{$t('federation.join.accept')}
				</Button>
				<a
					href={resolve('/')}
					class="inline-flex items-center text-sm text-muted-foreground underline"
				>
					{$t('federation.join.cancel')}
				</a>
			</div>
		{/if}
	{/if}
</div>
