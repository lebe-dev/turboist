<script lang="ts">
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import DayPartSection from '$lib/components/view/DayPartSection.svelte';
	import CompletedTodayFooter from '$lib/components/view/CompletedTodayFooter.svelte';
	import { activeDayPart, groupByDayPart } from '$lib/utils/viewGroup';
	import { parseIso, dayKeyInTz } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { toggleComplete, updateTaskFields } from '$lib/utils/taskActions';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartGroup } from '$lib/utils/viewGroup';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	let total = $state(0);
	let completedCount = $state(0);

	const list = useListMutator<Task>({
		onRemove: () => {
			total = Math.max(0, total - 1);
			completedCount += 1;
		}
	});
	const { mutator } = list;

	const dayParts = $derived(configStore.value?.dayParts);
	const tz = $derived(configStore.value?.timezone ?? null);
	const groups = $derived(groupByDayPart(list.items, dayParts));
	const active = $derived(activeDayPart(new Date(), dayParts, tz));


	const loader = usePageLoad(async (isValid) => {
		const ctxId = userStateStore.activeContextId ?? undefined;
		const [open, completed] = await Promise.all([
			viewsApi.today(getApiClient(), { contextId: ctxId }),
			viewsApi.completedToday(getApiClient(), { limit: 1, contextId: ctxId })
		]);
		if (!isValid()) return;
		list.items = open.items;
		total = open.total;
		completedCount = completed.total;
	}, { errorMessage: 'Failed to load today', autoLoad: false, initialLoading: true });

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});

	function isToday(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		return dayKeyInTz(dt, tz) === dayKeyInTz(new Date(), tz);
	}

	function bulkMove(group: DayPartGroup, targetPart: DayPart): void {
		for (const task of group.tasks) {
			void updateTaskFields(task, mutator, { dayPart: targetPart });
		}
	}

	function onUncompletedFromFooter(): void {
		completedCount = Math.max(0, completedCount - 1);
	}

</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0 && completedCount === 0}
		emptyIcon={SunIcon}
		emptyTitle="Nothing for today"
		emptyDescription="No tasks are scheduled for today. Enjoy the calm."
	>
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.part)}
				<DayPartSection
					part={group.part}
					label={group.label}
					interval={group.interval}
					count={group.tasks.length}
					active={group.part === active}
					onBulkMove={(targetPart) => bulkMove(group, targetPart)}
				>
					<TaskTree
						tasks={group.tasks}
						hideDayPart
						hideTodayBadge
						{mutator}
						belongs={isToday}
						onToggle={(t) => toggleComplete(t, mutator, { belongs: isToday })}
					/>
				</DayPartSection>
			{/each}

			<CompletedTodayFooter count={completedCount} onUncompleteOutside={onUncompletedFromFooter} />
		</div>
	</ViewContent>
</div>
