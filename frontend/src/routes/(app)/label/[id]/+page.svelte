<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { labels as labelsApi } from '$lib/api/endpoints/labels';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import type { Label, Task } from '$lib/api/types';
	import LabelHeader from '$lib/components/label/LabelHeader.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import LabelDialog from '$lib/components/dialog/LabelDialog.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const labelId = $derived(Number(page.params.id));

	let label = $state<Label | null>(null);
	let notFound = $state(false);
	let confirmDeleteOpen = $state(false);
	let editOpen = $state(false);

	const taskList = useListMutator<Task>();
	const mutator = taskList.mutator;

	const loader = usePageLoad(async (isValid) => {
		label = null;
		notFound = false;
		taskList.items = [];
		if (!Number.isFinite(labelId)) return;
		const client = getApiClient();
		const [l, ts] = await Promise.all([
			labelsApi.get(client, labelId),
			labelsApi.listTasks(client, labelId, { limit: 500 })
		]);
		if (!isValid()) return;
		label = l;
		taskList.items = ts.items;
	}, {
		errorMessage: 'Failed to load label',
		autoLoad: false,
		initialLoading: true,
		onError(err) {
			if (err instanceof ApiError && err.code === 'not_found') notFound = true;
		}
	});

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
			void goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete label'));
		}
	}

	$effect(() => {
		if (Number.isFinite(labelId)) void loader.refetch();
	});
</script>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !label}
	<div class="px-6 py-8 text-sm text-muted-foreground">Label not found</div>
{:else}
	<LabelHeader
		{label}
		onEdit={() => (editOpen = true)}
		onToggleFavourite={toggleFavourite}
		onDelete={() => (confirmDeleteOpen = true)}
	/>

	<div class="px-2 py-2">
		<ViewContent
			loading={false}
			isEmpty={taskList.items.length === 0}
			emptyIcon={TagIcon}
			emptyTitle="No tasks with this label"
			emptyDescription="Tag tasks with this label to see them here."
		>
			<TaskTree
				tasks={taskList.items}
				{mutator}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
			/>
		</ViewContent>
	</div>

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
