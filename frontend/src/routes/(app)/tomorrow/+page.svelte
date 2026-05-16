<script lang="ts">
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import { t } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import type { CalendarEvent, Task } from '$lib/api/types';
	import CalendarEventList from '$lib/components/calendar/CalendarEventList.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import DayPartSection from '$lib/components/view/DayPartSection.svelte';
	import { groupByDayPart } from '$lib/utils/viewGroup';
	import { calendarEventsOrEmpty } from '$lib/utils/calendar';
	import { parseIso, dayKeyInTz, dayStartUtcInTz, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { toggleComplete, updateTaskFields } from '$lib/utils/taskActions';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartGroup } from '$lib/utils/viewGroup';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	let total = $state(0);
	let calendarEvents = $state<CalendarEvent[]>([]);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const tz = $derived(configStore.value?.timezone ?? null);
	const dayParts = $derived(configStore.value?.dayParts);
	const groups = $derived(groupByDayPart(list.items, dayParts));


	const loader = usePageLoad(async (isValid) => {
		const todayKey = dayKeyInTz(new Date(), tz);
		const tomorrowKey = shiftDayKey(todayKey, 1);
		const start = toIsoUtc(dayStartUtcInTz(tomorrowKey, tz));
		const end = toIsoUtc(dayStartUtcInTz(shiftDayKey(tomorrowKey, 1), tz));
		const res = await viewsApi.tomorrow(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
		void loadCalendarEvents(start, end, isValid);
	}, { errorMessage: $t('page.tomorrow.errorLoading'), autoLoad: false, initialLoading: true });

	async function loadCalendarEvents(
		start: string,
		end: string,
		isValid: () => boolean
	): Promise<void> {
		const events = await calendarEventsOrEmpty(calendarsApi.events(getApiClient(), start, end));
		if (isValid()) calendarEvents = events;
	}

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});

	function bulkMove(group: DayPartGroup, targetPart: DayPart): void {
		for (const task of group.tasks) {
			void updateTaskFields(task, mutator, { dayPart: targetPart });
		}
	}

	function isTomorrow(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		const tomorrowKey = shiftDayKey(dayKeyInTz(new Date(), tz), 1);
		return dayKeyInTz(dt, tz) === tomorrowKey;
	}

</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0 && calendarEvents.length === 0}
		emptyIcon={SunHorizonIcon}
		emptyTitle={$t('page.tomorrow.emptyTitle')}
		emptyDescription={$t('page.tomorrow.emptyDescription')}
	>
		<div class="flex flex-col gap-4 py-2">
			<CalendarEventList events={calendarEvents} timezone={tz} />

			{#each groups as group (group.part)}
				<DayPartSection
					part={group.part}
					label={group.label}
					interval={group.interval}
					count={group.tasks.length}
					active={true}
					onBulkMove={(targetPart) => bulkMove(group, targetPart)}
				>
					<TaskTree
						tasks={group.tasks}
						hideTomorrowBadge
						{mutator}
						belongs={isTomorrow}
						onToggle={(t) => toggleComplete(t, mutator, { belongs: isTomorrow })}
					/>
				</DayPartSection>
			{/each}
		</div>
	</ViewContent>
</div>
