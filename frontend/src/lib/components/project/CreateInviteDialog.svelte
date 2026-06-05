<script lang="ts">
	import { toast } from 'svelte-sonner';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import { t } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { federation as federationApi } from '$lib/api/endpoints/federation';
	import type {
		CreateInviteRequest,
		CreateInviteResponse,
		FederationPermission
	} from '$lib/api/types';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	let {
		open = $bindable(false),
		projectId,
		// onCreated fires once with the freshly minted invite so the parent can
		// keep the link in memory for the session (the secret is never re-served,
		// so this is the only chance to enable copy-from-list — US-1.3 AC4/AC5).
		onCreated
	}: {
		open?: boolean;
		projectId: number;
		onCreated?: (invite: CreateInviteResponse) => void;
	} = $props();

	const PERMISSION_OPTIONS: FederationPermission[] = ['read', 'write'];
	// expiry presets in hours; 0 means "no expiry override" is not offered — the
	// owner always picks one of these, server still defaults to 7d if absent.
	const EXPIRY_OPTIONS: Array<{ key: string; hours: number }> = [
		{ key: '1h', hours: 1 },
		{ key: '1d', hours: 24 },
		{ key: '7d', hours: 24 * 7 }
	];
	const MAX_USES_OPTIONS = [1, 5, 10];

	let permissions = $state<FederationPermission>('write');
	let expiryHours = $state(24 * 7);
	let maxUses = $state(1);
	let creating = $state(false);
	let created = $state<CreateInviteResponse | null>(null);

	function reset() {
		permissions = 'write';
		expiryHours = 24 * 7;
		maxUses = 1;
		created = null;
	}

	$effect(() => {
		if (open) reset();
	});

	async function onCreate() {
		if (creating) return;
		const client = getApiClient();
		if (!client) return;
		creating = true;
		try {
			const body: CreateInviteRequest = { permissions };
			if (maxUses > 0) body.maxUses = maxUses;
			const expiresAt = new Date(Date.now() + expiryHours * 3600 * 1000);
			body.expiresAt = expiresAt.toISOString().replace(/\.\d{3}Z$/, '.000Z');
			created = await federationApi.createInvite(client, projectId, body);
			onCreated?.(created);
		} catch (err) {
			toast.error(describeError(err, $t('federation.invite.createFailed')));
		} finally {
			creating = false;
		}
	}

	async function copyLink() {
		if (!created) return;
		try {
			await navigator.clipboard.writeText(created.link);
			toast.success($t('federation.invite.copied'));
		} catch {
			// clipboard may be blocked; the link is selectable manually
		}
	}

	function close() {
		open = false;
	}
</script>

<AlertDialog.Root
	bind:open
	onOpenChange={(o) => {
		if (!o) reset();
	}}
>
	<AlertDialog.Content size="lg">
		{#if created}
			<AlertDialog.Header>
				<AlertDialog.Title>{$t('federation.invite.createdTitle')}</AlertDialog.Title>
				<AlertDialog.Description>{$t('federation.invite.warningOnce')}</AlertDialog.Description>
			</AlertDialog.Header>
			<div class="flex items-center gap-2">
				<code
					class="flex-1 break-all rounded-md border border-border bg-muted/40 px-2 py-1.5 font-mono text-xs"
					>{created.link}</code
				>
				<Button type="button" variant="outline" size="sm" onclick={copyLink}>
					<CopyIcon class="size-4" />
					{$t('federation.invite.copy')}
				</Button>
			</div>
			<AlertDialog.Footer>
				<AlertDialog.Action onclick={close}>{$t('federation.invite.done')}</AlertDialog.Action>
			</AlertDialog.Footer>
		{:else}
			<AlertDialog.Header>
				<AlertDialog.Title>{$t('federation.invite.title')}</AlertDialog.Title>
				<AlertDialog.Description>{$t('federation.invite.description')}</AlertDialog.Description>
			</AlertDialog.Header>

			<form
				class="flex flex-col gap-4"
				onsubmit={(e) => {
					e.preventDefault();
					onCreate();
				}}
			>
				<div class="flex flex-col gap-1.5">
					<Label>{$t('federation.invite.permissions')}</Label>
					<div class="flex flex-wrap gap-2">
						{#each PERMISSION_OPTIONS as p (p)}
							<Button
								type="button"
								variant={permissions === p ? 'default' : 'outline'}
								size="sm"
								disabled={creating}
								onclick={() => (permissions = p)}
							>
								{$t(`federation.invite.permission.${p}`)}
							</Button>
						{/each}
					</div>
				</div>

				<div class="flex flex-col gap-1.5">
					<Label>{$t('federation.invite.expiry')}</Label>
					<div class="flex flex-wrap gap-2">
						{#each EXPIRY_OPTIONS as opt (opt.key)}
							<Button
								type="button"
								variant={expiryHours === opt.hours ? 'default' : 'outline'}
								size="sm"
								disabled={creating}
								onclick={() => (expiryHours = opt.hours)}
							>
								{$t(`federation.invite.expiryOption.${opt.key}`)}
							</Button>
						{/each}
					</div>
				</div>

				<div class="flex flex-col gap-1.5">
					<Label for="invite-max-uses">{$t('federation.invite.maxUses')}</Label>
					<div class="flex flex-wrap items-center gap-2">
						{#each MAX_USES_OPTIONS as n (n)}
							<Button
								type="button"
								variant={maxUses === n ? 'default' : 'outline'}
								size="sm"
								disabled={creating}
								onclick={() => (maxUses = n)}
							>
								{n}
							</Button>
						{/each}
						<Input
							id="invite-max-uses"
							type="number"
							min={1}
							bind:value={maxUses}
							disabled={creating}
							class="w-20"
						/>
					</div>
				</div>

				<AlertDialog.Footer>
					<AlertDialog.Cancel disabled={creating}>{$t('common.cancel')}</AlertDialog.Cancel>
					<Button type="submit" disabled={creating}>
						{creating ? $t('federation.invite.creating') : $t('federation.invite.create')}
					</Button>
				</AlertDialog.Footer>
			</form>
		{/if}
	</AlertDialog.Content>
</AlertDialog.Root>
