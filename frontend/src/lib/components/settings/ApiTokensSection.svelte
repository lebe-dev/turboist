<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import ArrowSquareOutIcon from 'phosphor-svelte/lib/ArrowSquareOut';
	import { t } from '$lib/i18n';
	import { getApiClient, apiTokens, type APIToken, type APITokenWithSecret } from '$lib/api';
	import { describeError } from '$lib/utils/taskActions';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	let tokens = $state<APIToken[]>([]);
	let loading = $state(true);
	let creating = $state(false);
	let newName = $state('');
	let createdToken = $state<APITokenWithSecret | null>(null);
	let pendingDeleteId = $state<number | null>(null);
	let deleteOpen = $state(false);

	onMount(async () => {
		await load();
	});

	async function load() {
		const client = getApiClient();
		if (!client) return;
		loading = true;
		try {
			tokens = await apiTokens.list(client);
		} catch (err) {
			toast.error(describeError(err, $t('settings.api.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function onGenerate() {
		const name = newName.trim();
		if (!name || creating) return;
		const client = getApiClient();
		if (!client) return;
		creating = true;
		try {
			const created = await apiTokens.create(client, name);
			createdToken = created;
			tokens = [
				{ id: created.id, name: created.name, createdAt: created.createdAt },
				...tokens
			];
			newName = '';
		} catch (err) {
			toast.error(describeError(err, $t('settings.api.createFailed')));
		} finally {
			creating = false;
		}
	}

	function askDelete(id: number) {
		pendingDeleteId = id;
		deleteOpen = true;
	}

	async function onConfirmDelete() {
		if (pendingDeleteId == null) return;
		const client = getApiClient();
		if (!client) return;
		const id = pendingDeleteId;
		try {
			await apiTokens.delete(client, id);
			tokens = tokens.filter((tk) => tk.id !== id);
		} catch (err) {
			toast.error(describeError(err, $t('settings.api.deleteFailed')));
		} finally {
			pendingDeleteId = null;
			deleteOpen = false;
		}
	}

	async function copyToken() {
		if (!createdToken) return;
		try {
			await navigator.clipboard.writeText(createdToken.token);
			toast.success($t('settings.api.copied'));
		} catch {
			// clipboard may be blocked; user can still select manually
		}
	}

	function closeCreatedModal() {
		createdToken = null;
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.api.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.api.description')}</p>
		</div>
		<a
			href="https://github.com/lebe-dev/turboist/blob/main/API.md"
			target="_blank"
			rel="noopener noreferrer"
			class="flex shrink-0 items-center gap-1 text-xs text-muted-foreground underline underline-offset-2 transition-colors hover:text-foreground"
		>
			{$t('settings.api.docsLink')}
			<ArrowSquareOutIcon class="size-3.5 shrink-0" />
		</a>
	</div>

	<form
		class="flex flex-col gap-2 sm:flex-row sm:items-center"
		onsubmit={(e) => {
			e.preventDefault();
			onGenerate();
		}}
	>
		<Input
			type="text"
			placeholder={$t('settings.api.namePlaceholder')}
			bind:value={newName}
			disabled={creating}
			maxlength={64}
			class="sm:max-w-xs"
		/>
		<Button type="submit" variant="secondary" disabled={creating || newName.trim() === ''}>
			<PlusIcon class="size-4" />
			{creating ? $t('settings.api.generating') : $t('settings.api.generate')}
		</Button>
	</form>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if tokens.length === 0}
		<p class="text-xs text-muted-foreground">{$t('settings.api.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each tokens as tk (tk.id)}
				<li
					class="flex items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-2"
				>
					<div class="flex flex-col">
						<span class="text-sm font-medium">{tk.name}</span>
						<span class="text-xs text-muted-foreground">
							{$t('settings.api.created')}: {formatDate(tk.createdAt)}
						</span>
					</div>
					<button
						type="button"
						class="rounded-md p-2 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-destructive"
						aria-label={$t('settings.api.delete')}
						onclick={() => askDelete(tk.id)}
					>
						<TrashIcon class="size-4" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<AlertDialog.Root bind:open={deleteOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.api.confirmDeleteTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.api.confirmDeleteDescription')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onConfirmDelete}>
				{$t('settings.api.confirmDeleteAction')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root
	open={createdToken !== null}
	onOpenChange={(open) => {
		if (!open) closeCreatedModal();
	}}
>
	<AlertDialog.Content size="lg">
		<AlertDialog.Header>
			<AlertDialog.Title>{createdToken?.name ?? ''}</AlertDialog.Title>
			<AlertDialog.Description>{$t('settings.api.warningOnce')}</AlertDialog.Description>
		</AlertDialog.Header>
		<div class="flex items-center gap-2">
			<code
				class="flex-1 break-all rounded-md border border-border bg-muted/40 px-2 py-1.5 font-mono text-xs"
				>{createdToken?.token ?? ''}</code
			>
			<Button type="button" variant="outline" size="sm" onclick={copyToken}>
				<CopyIcon class="size-4" />
				{$t('settings.api.copy')}
			</Button>
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Action onclick={closeCreatedModal}>
				{$t('settings.api.close')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
