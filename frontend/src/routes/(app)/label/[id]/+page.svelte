<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import { getApiClient } from '$lib/api/client';
	import { labels as labelsApi } from '$lib/api/endpoints/labels';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import type { Label, Task } from '$lib/api/types';
	import LabelHeader from '$lib/components/label/LabelHeader.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import TaskEditorSheet from '$lib/components/task/TaskEditorSheet.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import LabelDialog from '$lib/components/dialog/LabelDialog.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		saveEdit,
		describeError
	} from '$lib/utils/taskActions';

	const labelId = $derived(Number(page.params.id));

	let label = $state<Label | null>(null);
	let tasks = $state<Task[]>([]);
	let loading = $state(true);
	let editing = $state<Task | null>(null);
	let editorOpen = $state(false);
	let confirmDeleteOpen = $state(false);
	let editOpen = $state(false);

	const mutator = {
		replace(t: Task) {
			tasks = tasks.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			tasks = tasks.filter((x) => x.id !== id);
		}
	};

	let requestSeq = 0;

	async function load(): Promise<void> {
		const my = ++requestSeq;
		loading = true;
		try {
			const client = getApiClient();
			const [l, ts] = await Promise.all([
				labelsApi.get(client, labelId),
				labelsApi.listTasks(client, labelId, { limit: 500 })
			]);
			if (my !== requestSeq) return;
			label = l;
			tasks = ts.items;
		} catch (err) {
			if (my !== requestSeq) return;
			toast.error(describeError(err, 'Failed to load label'));
		} finally {
			if (my === requestSeq) loading = false;
		}
	}

	async function toggleFavourite() {
		if (!label) return;
		try {
			const updated = await labelsApi.update(getApiClient(), label.id, {
				isFavourite: !label.isFavourite
			});
			label = updated;
			labelsStore.upsert(updated);
		} catch (err) {
			toast.error(describeError(err, 'Failed to update label'));
		}
	}

	async function deleteLabel() {
		if (!label) return;
		try {
			await labelsApi.remove(getApiClient(), label.id);
			labelsStore.remove(label.id);
			toast.success('Label deleted');
			goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete label'));
		}
	}

	function openEditor(task: Task): void {
		editing = task;
		editorOpen = true;
	}

	$effect(() => {
		if (Number.isFinite(labelId)) load();
	});

	onMount(() => undefined);
</script>

{#if loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if !label}
	<div class="px-6 py-8 text-sm text-muted-foreground">Label not found</div>
{:else}
	<LabelHeader
		{label}
		onEdit={() => (editOpen = true)}
		onToggleFavourite={toggleFavourite}
		onDelete={() => (confirmDeleteOpen = true)}
	/>

	<div class="px-2 py-2">
		{#if tasks.length === 0}
			<EmptyState
				icon={TagIcon}
				title="No tasks with this label"
				description="Tag tasks with this label to see them here."
			/>
		{:else}
			<TaskTree
				{tasks}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
				onPinToggle={(t) => togglePin(t, mutator)}
				onDelete={(t) => deleteTask(t, mutator)}
				onEdit={openEditor}
			/>
		{/if}
	</div>

	<TaskEditorSheet
		bind:open={editorOpen}
		task={editing}
		onSubmit={(id, payload) =>
			saveEdit(id, payload, mutator, (t) => t.labels.some((l) => l.id === labelId))}
	/>
	<LabelDialog
		bind:open={editOpen}
		initial={label}
		onSaved={(l) => (label = l)}
	/>
	<ConfirmDestructiveDialog
		bind:open={confirmDeleteOpen}
		title="Delete label?"
		description="The label will be removed from all tasks. This cannot be undone."
		onConfirm={deleteLabel}
	/>
{/if}
