<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import TaskEditorSheet from '$lib/components/task/TaskEditorSheet.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		saveEdit,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let editing = $state<Task | null>(null);
	let editorOpen = $state(false);

	const limit = $derived(configStore.value?.backlog.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			total = Math.max(0, total - 1);
		}
	};

	async function load(): Promise<void> {
		loading = true;
		try {
			const res = await viewsApi.backlog(getApiClient());
			items = res.items;
			total = res.total;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load backlog'));
		} finally {
			loading = false;
		}
	}

	function openEditor(task: Task): void {
		editing = task;
		editorOpen = true;
	}

	onMount(load);
</script>

<ViewHeader title="Backlog" subtitle={loading ? 'Loading…' : 'Plans for later'}>
	{#snippet actions()}
		{#if limit !== null}
			<LimitBadge count={total} {limit} />
		{/if}
	{/snippet}
	{#snippet banner()}
		{#if exceeded && limit !== null}
			<div
				class="rounded border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
			>
				Backlog limit reached ({total}/{limit}). Move tasks to a week or complete them before
				adding more.
			</div>
		{/if}
	{/snippet}
</ViewHeader>

<div class="px-2 py-2">
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0}
		<EmptyState
			icon={StackIcon}
			title="Backlog is empty"
			description="Park tasks here when they're not actionable yet."
		/>
	{:else}
		<TaskTree
			tasks={items}
			onToggle={(t) => toggleComplete(t, mutator)}
			onPinToggle={(t) => togglePin(t, mutator)}
			onDelete={(t) => deleteTask(t, mutator)}
			onEdit={openEditor}
		/>
	{/if}
</div>

<TaskEditorSheet
	bind:open={editorOpen}
	task={editing}
	onSubmit={(id, payload) => saveEdit(id, payload, mutator)}
/>
