<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import { t } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { CalendarEvent, Task } from '$lib/api/types';
	import CalendarEventList from '$lib/components/calendar/CalendarEventList.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import LimitReachedBanner from '$lib/components/view/LimitReachedBanner.svelte';
	import GroupHeader from '$lib/components/view/GroupHeader.svelte';
	import { groupByDay } from '$lib/utils/viewGroup';
	import { groupCalendarEventsByDay } from '$lib/utils/calendar';
	import { dayKeyInTz, dayStartUtcInTz, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';


	let total = $state(0);
	let calendarEvents = $state<CalendarEvent[]>([]);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const groups = $derived(groupByDay(list.items, configStore.value?.timezone ?? null));
	const eventGroups = $derived(
		groupCalendarEventsByDay(calendarEvents, {
			today: $t('common.today'),
			tomorrow: $t('common.tomorrow'),
			yesterday: $t('common.yesterday')
		}, configStore.value?.timezone ?? null)
	);
	const limit = $derived(configStore.value?.weekly.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);

	const loader = usePageLoad(async (isValid) => {
		const tz = configStore.value?.timezone ?? null;
		const todayKey = dayKeyInTz(new Date(), tz);
		const start = toIsoUtc(dayStartUtcInTz(todayKey, tz));
		const end = toIsoUtc(dayStartUtcInTz(shiftDayKey(todayKey, 7), tz));
		const [res, events] = await Promise.all([
			viewsApi.week(getApiClient(), {
				contextId: userStateStore.activeContextId ?? undefined
			}),
			settingsStore.calendarEnabled
				? calendarsApi.events(getApiClient(), start, end).catch(() => [])
				: Promise.resolve([])
		]);
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
		calendarEvents = events;
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
		isEmpty={list.items.length === 0 && calendarEvents.length === 0}
		emptyIcon={CalendarIcon}
		emptyTitle={$t('page.week.emptyTitle')}
		emptyDescription={$t('page.week.emptyDescription')}
	>
		<div class="flex flex-col gap-4 py-2">
			{#each eventGroups as group (group.dayKey)}
				<section>
					<GroupHeader label={group.label} />
					<CalendarEventList events={group.events} timezone={configStore.value?.timezone ?? null} compact />
				</section>
			{/each}

			{#each groups as group (group.dayKey)}
				<section>
					<GroupHeader label={group.label} />
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
