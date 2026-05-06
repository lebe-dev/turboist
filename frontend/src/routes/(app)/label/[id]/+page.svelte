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
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { t } from '$lib/i18n';


	const labelId = $derived(Number(page.params.id));

	let label = $state<Label | null>(null);
	$effect(() => { if (label) viewFilterStore.setTitle(label.name); });
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
		errorMessage: $t('page.label.errorLoading'),
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
			toast.error(describeError(err, $t('page.label.failedUpdate')));
		}
	}

	async function togglePrivate() {
		if (!label) return;
		try {
			const updated = await labelsApi.update(getApiClient(), label.id, {
				isPrivate: !label.isPrivate
			});
			label = updated;
			labelsStore.upsert(updated);
			toast.success($t('common.privacyUpdated'));
		} catch (err) {
			toast.error(describeError(err, $t('page.label.failedUpdatePrivacy')));
		}
	}

	$effect(() => {
		if (label && label.isPrivate && settingsStore.publicView) {
			toast.info($t('common.privateHidden'));
			void goto(resolve('/today'));
		}
	});

	async function deleteLabel() {
		if (!label) return;
		try {
			await labelsApi.remove(getApiClient(), label.id);
			labelsStore.remove(label.id);
			toast.success($t('page.label.deleted'));
			void goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, $t('page.label.failedDelete')));
		}
	}

	$effect(() => {
		if (Number.isFinite(labelId)) void loader.refetch();
	});
</script>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('app.loading')}</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !label}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('page.label.notFound')}</div>
{:else}
	<LabelHeader
		{label}
		onEdit={() => (editOpen = true)}
		onToggleFavourite={toggleFavourite}
		onTogglePrivate={togglePrivate}
		onDelete={() => (confirmDeleteOpen = true)}
	/>

	<div class="px-2 py-2">
		<ViewContent
			loading={false}
			isEmpty={taskList.items.length === 0}
			emptyIcon={TagIcon}
			emptyTitle={$t('page.label.emptyTitle')}
			emptyDescription={$t('page.label.emptyDescription')}
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
		title={$t('page.label.confirmDeleteTitle')}
		description={$t('page.label.confirmDeleteDesc')}
		onConfirm={deleteLabel}
	/>
{/if}
