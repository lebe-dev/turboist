<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import { t } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import LimitReachedBanner from '$lib/components/view/LimitReachedBanner.svelte';
	import GroupHeader from '$lib/components/view/GroupHeader.svelte';
	import { groupByDay } from '$lib/utils/viewGroup';
	import { dayKeyInTz, parseIso, weekRangeKeys } from '$lib/utils/format';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';


	let total = $state(0);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const tz = $derived(configStore.value?.timezone ?? null);
	const groups = $derived(groupByDay(list.items, tz));
	const limit = $derived(configStore.value?.weekly.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);
	const weekRange = $derived(weekRangeKeys(new Date(), tz));

	function dueInWeek(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		const key = dayKeyInTz(dt, tz);
		return key >= weekRange.startKey && key < weekRange.endKey;
	}

	function belongs(t: Task): boolean {
		return t.planState === 'week' || dueInWeek(t);
	}

	const loader = usePageLoad(async (isValid) => {
		const res = await viewsApi.week(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
	}, { errorMessage: $t('page.week.errorLoading'), autoLoad: false, initialLoading: true });

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
			<LimitReachedBanner
				message={$t('page.week.limitReached', { values: { total, limit } })}
			/>
		{/if}
	{/snippet}
</ViewHeader>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={CalendarIcon}
		emptyTitle={$t('page.week.emptyTitle')}
		emptyDescription={$t('page.week.emptyDescription')}
	>
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.dayKey)}
				<section>
					<GroupHeader label={group.label} />
					<TaskTree
						tasks={group.tasks}
						showUnplannedBadge
						{mutator}
						{belongs}
						onToggle={(t) => toggleComplete(t, mutator)}
					/>
				</section>
			{/each}
		</div>
	</ViewContent>
</div>
