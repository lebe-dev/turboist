<script lang="ts">
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { groupByCompletedDay } from '$lib/utils/viewGroup';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const DAYS = 30;

	const list = useListMutator<Task>();
	const { mutator } = list;

	const groups = $derived(groupByCompletedDay(list.items, configStore.value?.timezone ?? null));

	const loader = usePageLoad(
		async (isValid) => {
			const res = await viewsApi.completed(getApiClient(), {
				days: DAYS,
				limit: 500,
				contextId: userStateStore.activeContextId ?? undefined
			});
			if (!isValid()) return;
			list.items = res.items;
		},
		{ errorMessage: 'Failed to load completed', autoLoad: false, initialLoading: true }
	);

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});
</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={CheckCircleIcon}
		emptyTitle="Nothing completed yet"
		emptyDescription="Tasks you complete will show up here, grouped by day."
	>
		<div class="flex flex-col py-2">
			{#each groups as group, i (group.dayKey)}
				{#if i > 0}
					<hr class="my-4 border-t border-border" />
				{/if}
				<section>
					<h2 class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
						{group.label}
					</h2>
					<TaskTree
						tasks={group.tasks}
						hideDue
						{mutator}
						belongs={(t) => t.status === 'completed'}
						onToggle={(t) => toggleComplete(t, mutator)}
					/>
				</section>
			{/each}
		</div>
	</ViewContent>
</div>
