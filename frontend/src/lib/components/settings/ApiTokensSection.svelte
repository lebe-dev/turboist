<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import ArrowSquareOutIcon from 'phosphor-svelte/lib/ArrowSquareOut';
	import { t } from '$lib/i18n';
	import {
		getApiClient,
		apiTokens,
		SCOPE_RESOURCES,
		type APIToken,
		type APITokenWithSecret
	} from '$lib/api';
	import { describeError } from '$lib/utils/taskActions';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Badge } from '$lib/components/ui/badge';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	type ResourceKey = (typeof SCOPE_RESOURCES)[number]['resource'];
	type ScopeState = Record<ResourceKey, { read: boolean; write: boolean }>;

	function makeEmptyScopes(): ScopeState {
		const result = {} as ScopeState;
		for (const r of SCOPE_RESOURCES) {
			result[r.resource] = { read: false, write: false };
		}
		return result;
	}

	let tokens = $state<APIToken[]>([]);
	let loading = $state(true);
	let creating = $state(false);
	let newName = $state('');
	let createdToken = $state<APITokenWithSecret | null>(null);
	let pendingDeleteId = $state<number | null>(null);
	let deleteOpen = $state(false);

	let fullAccess = $state(false);
	let scopesState = $state<ScopeState>(makeEmptyScopes());

	const hasAnyScope = $derived(
		fullAccess ||
			SCOPE_RESOURCES.some((r) => scopesState[r.resource].read || scopesState[r.resource].write)
	);

	function normalizeScopes(): string[] {
		if (fullAccess) return ['*'];
		const result: string[] = [];
		for (const r of SCOPE_RESOURCES) {
			const s = scopesState[r.resource];
			if (s.write && r.hasWrite) {
				result.push(`${r.resource}:read`, `${r.resource}:write`);
			} else if (s.read) {
				result.push(`${r.resource}:read`);
			}
		}
		return Array.from(new Set(result));
	}

	function applyPresetFull() {
		fullAccess = true;
	}

	function applyPresetReadonly() {
		fullAccess = false;
		const next = makeEmptyScopes();
		for (const r of SCOPE_RESOURCES) {
			next[r.resource].read = true;
		}
		scopesState = next;
	}

	function applyPresetTasksFull() {
		fullAccess = false;
		const next = makeEmptyScopes();
		next.tasks.read = true;
		next.tasks.write = true;
		scopesState = next;
	}

	function onReadToggle(resource: ResourceKey, checked: boolean) {
		fullAccess = false;
		scopesState[resource].read = checked;
		if (!checked) {
			scopesState[resource].write = false;
		}
	}

	function onWriteToggle(resource: ResourceKey, checked: boolean) {
		fullAccess = false;
		scopesState[resource].write = checked;
		if (checked) {
			scopesState[resource].read = true;
		}
	}

	function resetForm() {
		newName = '';
		fullAccess = false;
		scopesState = makeEmptyScopes();
	}

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
		const scopes = normalizeScopes();
		if (scopes.length === 0) {
			toast.error($t('settings.api.scopes.emptyError'));
			return;
		}
		const client = getApiClient();
		if (!client) return;
		creating = true;
		try {
			const created = await apiTokens.create(client, name, scopes);
			createdToken = created;
			tokens = [
				{
					id: created.id,
					name: created.name,
					scopes: created.scopes,
					createdAt: created.createdAt
				},
				...tokens
			];
			resetForm();
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

	function scopeBadgeLabel(scope: string): string {
		if (scope === '*') return $t('settings.api.scopes.fullBadge');
		const [resource, action] = scope.split(':');
		const resourceLabel = $t(`settings.api.scopes.resources.${resource}`);
		const actionLabel = action === 'write' ? $t('settings.api.scopes.write') : $t('settings.api.scopes.read');
		return `${resourceLabel}: ${actionLabel}`;
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
		class="flex flex-col gap-3"
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

		<div class="flex flex-col gap-2">
			<div class="flex flex-wrap items-center gap-2">
				<span class="text-xs font-medium text-muted-foreground">
					{$t('settings.api.scopes.presetsLabel')}
				</span>
				<Button
					type="button"
					variant={fullAccess ? 'default' : 'outline'}
					size="sm"
					onclick={applyPresetFull}
					disabled={creating}
				>
					{$t('settings.api.scopes.presets.full')}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onclick={applyPresetReadonly}
					disabled={creating}
				>
					{$t('settings.api.scopes.presets.readonly')}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onclick={applyPresetTasksFull}
					disabled={creating}
				>
					{$t('settings.api.scopes.presets.tasksFull')}
				</Button>
			</div>

			<div class="overflow-hidden rounded-md border border-border">
				<table class="w-full text-sm">
					<thead class="bg-muted/40 text-xs text-muted-foreground">
						<tr>
							<th class="px-3 py-1.5 text-left font-medium">
								{$t('settings.api.scopes.headers.resource')}
							</th>
							<th class="w-20 px-3 py-1.5 text-center font-medium">
								{$t('settings.api.scopes.headers.read')}
							</th>
							<th class="w-20 px-3 py-1.5 text-center font-medium">
								{$t('settings.api.scopes.headers.write')}
							</th>
						</tr>
					</thead>
					<tbody>
						{#each SCOPE_RESOURCES as r (r.resource)}
							{@const state = scopesState[r.resource]}
							{@const readChecked = fullAccess || state.read}
							{@const writeChecked = r.hasWrite && (fullAccess || state.write)}
							{@const readDisabled =
								creating || fullAccess || (r.hasWrite && state.write)}
							{@const writeDisabled = creating || fullAccess}
							<tr class="border-t border-border">
								<td class="px-3 py-1.5">
									{$t(`settings.api.scopes.resources.${r.resource}`)}
								</td>
								<td class="px-3 py-1.5 text-center">
									<div class="flex justify-center">
										<Checkbox
											checked={readChecked}
											disabled={readDisabled}
											aria-label={$t(`settings.api.scopes.resources.${r.resource}`) +
												' — ' +
												$t('settings.api.scopes.headers.read')}
											onCheckedChange={(v) => onReadToggle(r.resource, v === true)}
										/>
									</div>
								</td>
								<td class="px-3 py-1.5 text-center">
									{#if r.hasWrite}
										<div class="flex justify-center">
											<Checkbox
												checked={writeChecked}
												disabled={writeDisabled}
												aria-label={$t(`settings.api.scopes.resources.${r.resource}`) +
													' — ' +
													$t('settings.api.scopes.headers.write')}
												onCheckedChange={(v) => onWriteToggle(r.resource, v === true)}
											/>
										</div>
									{:else}
										<span class="text-muted-foreground">—</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>

		<div>
			<Button
				type="submit"
				variant="secondary"
				disabled={creating || newName.trim() === '' || !hasAnyScope}
			>
				<PlusIcon class="size-4" />
				{creating ? $t('settings.api.generating') : $t('settings.api.generate')}
			</Button>
		</div>
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
					<div class="flex min-w-0 flex-col gap-1">
						<span class="text-sm font-medium">{tk.name}</span>
						<span class="text-xs text-muted-foreground">
							{$t('settings.api.created')}: {formatDate(tk.createdAt)}
						</span>
						{#if tk.scopes && tk.scopes.length > 0}
							<div class="flex flex-wrap gap-1">
								{#if tk.scopes.includes('*')}
									<Badge variant="default">{$t('settings.api.scopes.fullBadge')}</Badge>
								{:else}
									{#each tk.scopes as scope (scope)}
										<Badge variant="secondary">{scopeBadgeLabel(scope)}</Badge>
									{/each}
								{/if}
							</div>
						{/if}
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
