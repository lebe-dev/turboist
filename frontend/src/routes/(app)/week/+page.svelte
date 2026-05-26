<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import FunnelIcon from 'phosphor-svelte/lib/Funnel';
	import { Button } from '$lib/components/ui/button';
	import { t, locale } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { CalendarEvent, Task } from '$lib/api/types';
	import CalendarEventItem from '$lib/components/calendar/CalendarEventItem.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import LimitReachedBanner from '$lib/components/view/LimitReachedBanner.svelte';
	import GroupHeader from '$lib/components/view/GroupHeader.svelte';
	import CompletedThisWeekFooter from '$lib/components/view/CompletedThisWeekFooter.svelte';
	import { groupByDay } from '$lib/utils/viewGroup';
	import { calendarEventsOrEmpty, groupCalendarEventsByDay, isPastCalendarEvent } from '$lib/utils/calendar';
	import {
		dayKeyInTz,
		dayStartUtcInTz,
		daysBetweenKeys,
		formatDayKeyRange,
		parseIso,
		toIsoUtc,
		weekRangeKeys
	} from '$lib/utils/format';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { nowStore } from '$lib/stores/now.svelte';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { queryWeek } from '$lib/offline/views';


	const ONLY_PLANNED_KEY = 'turboist:week:only-planned';

	function loadOnlyPlanned(): boolean {
		if (typeof localStorage === 'undefined') return false;
		return localStorage.getItem(ONLY_PLANNED_KEY) === '1';
	}

	let total = $state(0);
	let calendarEvents = $state<CalendarEvent[]>([]);
	let now = $state(new Date());
	let onlyPlanned = $state(loadOnlyPlanned());

	function toggleOnlyPlanned(): void {
		onlyPlanned = !onlyPlanned;
		if (typeof localStorage !== 'undefined') {
			localStorage.setItem(ONLY_PLANNED_KEY, onlyPlanned ? '1' : '0');
		}
	}

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const tz = $derived(configStore.value?.timezone ?? null);
	const activeCalendarEvents = $derived(
		settingsStore.calendarHidePastEvents
			? calendarEvents.filter((event) => !isPastCalendarEvent(event, now, tz))
			: calendarEvents
	);
	const visibleTasks = $derived(
		onlyPlanned ? list.items.filter((t) => t.planState === 'week') : list.items
	);
	const groups = $derived(groupByDay(visibleTasks, tz, nowStore.now));
	const eventGroups = $derived(
		groupCalendarEventsByDay(activeCalendarEvents, {
			today: $t('common.today'),
			tomorrow: $t('common.tomorrow'),
			yesterday: $t('common.yesterday')
		}, tz)
	);
	const combinedGroups = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const byKey = new Map<
			string,
			{
				dayKey: string;
				label: string;
				tasks: Task[];
				events: CalendarEvent[];
			}
		>();
		for (const group of groups) {
			byKey.set(group.dayKey, {
				dayKey: group.dayKey,
				label: group.label,
				tasks: group.tasks,
				events: []
			});
		}
		for (const group of eventGroups) {
			const existing = byKey.get(group.dayKey);
			if (existing) existing.events = group.events;
			else {
				byKey.set(group.dayKey, {
					dayKey: group.dayKey,
					label: group.label,
					tasks: [],
					events: group.events
				});
			}
		}
		return [...byKey.values()].sort((a, b) => {
			if (a.dayKey === 'no-date') return 1;
			if (b.dayKey === 'no-date') return -1;
			return a.dayKey < b.dayKey ? -1 : a.dayKey > b.dayKey ? 1 : 0;
		});
	});
	const limit = $derived(configStore.value?.weekly.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);
	const weekRange = $derived(weekRangeKeys(nowStore.now, tz));
	const weekRangeLabel = $derived(
		formatDayKeyRange(weekRange.startKey, weekRange.endKey, $locale, tz)
	);
	const daysPassed = $derived(daysBetweenKeys(weekRange.startKey, nowStore.todayKey));
	const daysLeftCount = $derived(daysPassed >= 1 ? 7 - daysPassed : 0);

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
		const start = toIsoUtc(dayStartUtcInTz(weekRange.startKey, tz));
		const end = toIsoUtc(dayStartUtcInTz(weekRange.endKey, tz));
		const res = await viewsApi.week(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
		void loadCalendarEvents(start, end, isValid);
	}, {
		errorMessage: $t('page.week.errorLoading'),
		autoLoad: false,
		initialLoading: true,
		offlineFallback: async (isValid) => {
			const res = await queryWeek(tz, userStateStore.activeContextId ?? null);
			if (!isValid()) return;
			list.items = res.items;
			total = res.total;
			calendarEvents = [];
		}
	});

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

	let lastDayKey = $state(nowStore.todayKey);
	$effect(() => {
		const k = nowStore.todayKey;
		if (k !== lastDayKey) {
			lastDayKey = k;
			void loader.revalidate();
		}
	});

	useInvalidation(['tasks', 'calendar'], () => void loader.revalidate());

	$effect(() => {
		const timer = window.setInterval(() => {
			now = new Date();
		}, 60_000);
		return () => window.clearInterval(timer);
	});
</script>

<ViewHeader stackOnMobile>
	{#snippet meta()}
		<p class="text-xl font-semibold leading-tight tracking-tight text-foreground sm:text-2xl">
			{weekRangeLabel}
		</p>
		{#if daysLeftCount > 0}
			<p class="mt-1 text-sm text-muted-foreground">
				{$t('page.week.daysLeft', { values: { count: daysLeftCount } })}
			</p>
		{/if}
	{/snippet}
	{#snippet actions()}
		<Button
			variant={onlyPlanned ? 'default' : 'outline'}
			size="sm"
			onclick={toggleOnlyPlanned}
			aria-pressed={onlyPlanned}
			title={$t('page.week.onlyPlannedTooltip')}
			class={onlyPlanned ? 'ring-2 ring-primary/40' : ''}
		>
			<FunnelIcon weight={onlyPlanned ? 'fill' : 'regular'} />
			<span>{$t('page.week.onlyPlanned')}</span>
		</Button>
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
		isEmpty={visibleTasks.length === 0 && activeCalendarEvents.length === 0}
		emptyIcon={CalendarIcon}
		emptyTitle={$t('page.week.emptyTitle')}
		emptyDescription={$t('page.week.emptyDescription')}
	>
		<div class="flex flex-col gap-4 py-2">
			{#each combinedGroups as group (group.dayKey)}
				<section>
					<GroupHeader label={group.label} />
					{#each group.events as event (event.id)}
						<CalendarEventItem {event} timezone={tz} />
					{/each}
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
	{#if !loader.loading}
		<CompletedThisWeekFooter />
	{/if}
</div>
