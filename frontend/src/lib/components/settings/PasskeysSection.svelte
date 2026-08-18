<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import KeyIcon from 'phosphor-svelte/lib/Key';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { t, locale } from '$lib/i18n';
	import { getApiClient, passkeys, ApiError, type Passkey } from '$lib/api';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { isPasskeySupported, isPasskeyCancellation } from '$lib/webauthn';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	const authStore = getAuthStore();

	let items = $state<Passkey[]>([]);
	let loading = $state(true);
	let adding = $state(false);
	let newName = $state('');
	let removingId = $state<number | null>(null);
	let pendingRemoveId = $state<number | null>(null);
	let removeOpen = $state(false);
	let renamingId = $state<number | null>(null);
	let renameDraft = $state('');

	// Resolved once: the WebAuthn API either exists in this runtime or it does
	// not, and a passkey button that cannot open a prompt is worse than none.
	const supported = isPasskeySupported();

	onMount(load);

	async function load(): Promise<void> {
		const client = getApiClient();
		if (!client) return;
		loading = true;
		try {
			items = await passkeys.list(client);
		} catch (err) {
			toast.error(describeError(err, $t('settings.passkeys.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function onAdd(): Promise<void> {
		if (adding) return;
		adding = true;
		try {
			const created = await authStore.registerPasskey(newName.trim());
			items = [created, ...items];
			newName = '';
			toast.success($t('settings.passkeys.added'));
		} catch (err) {
			// Dismissing the platform sheet is a choice, not a failure.
			if (isPasskeyCancellation(err)) return;
			if (err instanceof ApiError && err.code === 'passkey_exists') {
				toast.error($t('settings.passkeys.duplicate'));
				return;
			}
			if (err instanceof ApiError && err.code === 'limit_exceeded') {
				toast.error($t('settings.passkeys.limitReached'));
				return;
			}
			toast.error(describeError(err, $t('settings.passkeys.addFailed')));
		} finally {
			adding = false;
		}
	}

	function startRename(item: Passkey): void {
		renamingId = item.id;
		renameDraft = item.name;
	}

	async function commitRename(): Promise<void> {
		const client = getApiClient();
		const id = renamingId;
		if (!client || id == null) return;
		const name = renameDraft.trim();
		renamingId = null;
		const current = items.find((p) => p.id === id);
		if (!name || !current || name === current.name) return;
		try {
			const updated = await passkeys.rename(client, id, name);
			items = items.map((p) => (p.id === id ? updated : p));
		} catch (err) {
			toast.error(describeError(err, $t('settings.passkeys.renameFailed')));
		}
	}

	function askRemove(id: number): void {
		pendingRemoveId = id;
		removeOpen = true;
	}

	async function onConfirmRemove(): Promise<void> {
		const client = getApiClient();
		if (!client || pendingRemoveId == null) return;
		const id = pendingRemoveId;
		removingId = id;
		try {
			await passkeys.remove(client, id);
			items = items.filter((p) => p.id !== id);
		} catch (err) {
			toast.error(describeError(err, $t('settings.passkeys.removeFailed')));
		} finally {
			removingId = null;
			pendingRemoveId = null;
			removeOpen = false;
		}
	}

	function formatDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleDateString($locale || 'en', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.passkeys.heading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.passkeys.description')}</p>
	</div>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if items.length === 0}
		<p class="text-xs text-muted-foreground">{$t('settings.passkeys.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each items as p (p.id)}
				<li
					class="flex items-start justify-between gap-3 rounded-md border border-border bg-background px-3 py-2.5"
				>
					<div class="flex min-w-0 flex-1 items-start gap-3">
						<KeyIcon class="size-4 shrink-0 text-muted-foreground" />
						<div class="flex min-w-0 flex-1 flex-col gap-0.5">
							{#if renamingId === p.id}
								<Input
									bind:value={renameDraft}
									class="h-7 text-sm"
									autofocus
									onblur={commitRename}
									onkeydown={(e: KeyboardEvent) => {
										if (e.key === 'Enter') void commitRename();
										if (e.key === 'Escape') renamingId = null;
									}}
								/>
							{:else}
								<button
									type="button"
									class="truncate text-left text-sm font-medium hover:underline"
									title={$t('settings.passkeys.rename')}
									onclick={() => startRename(p)}
								>
									{p.name}
								</button>
							{/if}
							<span class="text-xs text-muted-foreground">
								{$t('settings.passkeys.created')}: {formatDate(p.createdAt)}
							</span>
							<span class="text-[11px] text-muted-foreground/80">
								{p.lastUsedAt
									? `${$t('settings.passkeys.lastUsed')}: ${formatDate(p.lastUsedAt)}`
									: $t('settings.passkeys.neverUsed')}
							</span>
						</div>
					</div>
					<button
						type="button"
						class="inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-destructive/40 hover:bg-destructive/10 hover:text-destructive disabled:cursor-not-allowed disabled:opacity-60"
						onclick={() => askRemove(p.id)}
						disabled={removingId === p.id}
					>
						<TrashIcon class="size-3.5" />
						{removingId === p.id
							? $t('settings.passkeys.removing')
							: $t('settings.passkeys.remove')}
					</button>
				</li>
			{/each}
		</ul>
	{/if}

	{#if supported}
		<div class="flex flex-wrap items-center gap-2 pt-1">
			<Input
				bind:value={newName}
				class="h-8 max-w-56 text-sm"
				placeholder={$t('settings.passkeys.namePlaceholder')}
			/>
			<Button type="button" variant="outline" size="sm" onclick={onAdd} disabled={adding}>
				<PlusIcon class="size-4" />
				{adding ? $t('settings.passkeys.adding') : $t('settings.passkeys.add')}
			</Button>
		</div>
	{:else}
		<p class="text-xs text-muted-foreground">{$t('settings.passkeys.unsupported')}</p>
	{/if}
</section>

<AlertDialog.Root bind:open={removeOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.passkeys.confirmRemoveTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.passkeys.confirmRemoveDescription')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onConfirmRemove}>
				{$t('settings.passkeys.remove')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
