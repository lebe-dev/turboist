<script lang="ts">
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';


	let total = $state(0);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const limit = $derived(configStore.value?.backlog.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);

	const loader = usePageLoad(async (isValid) => {
		const res = await viewsApi.backlog(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
	}, { errorMessage: 'Failed to load backlog', autoLoad: false, initialLoading: true });

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});
</script>

<ViewHeader>
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
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={StackIcon}
		emptyTitle="Backlog is empty"
		emptyDescription="Park tasks here when they're not actionable yet."
	>
		<TaskTree
			tasks={list.items}
			{mutator}
			belongs={(t) => t.planState === 'backlog'}
			onToggle={(t) => toggleComplete(t, mutator)}
		/>
	</ViewContent>
</div>
