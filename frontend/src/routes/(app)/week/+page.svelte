<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import { groupByDay } from '$lib/utils/viewGroup';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';


	let total = $state(0);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const groups = $derived(groupByDay(list.items, configStore.value?.timezone ?? null));
	const limit = $derived(configStore.value?.weekly.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);

	const loader = usePageLoad(async (isValid) => {
		const res = await viewsApi.week(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
	}, { errorMessage: 'Failed to load week', autoLoad: false, initialLoading: true });

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
				Weekly limit reached ({total}/{limit}). Adding more tasks to the week will be rejected.
			</div>
		{/if}
	{/snippet}
</ViewHeader>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={CalendarIcon}
		emptyTitle="Week is empty"
		emptyDescription="Plan tasks for this week from Backlog or by setting a due date."
	>
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.dayKey)}
				<section>
					<h2 class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
						{group.label}
					</h2>
					<TaskTree
						tasks={group.tasks}
						{mutator}
						belongs={(t) => t.planState === 'week'}
						onToggle={(t) => toggleComplete(t, mutator)}
					/>
				</section>
			{/each}
		</div>
	</ViewContent>
</div>
