<script lang="ts">
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import type { CalendarEvent, Task } from '$lib/api/types';
	import { t } from '$lib/i18n';
	import CalendarEventList from '$lib/components/calendar/CalendarEventList.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import DayPartSection from '$lib/components/view/DayPartSection.svelte';
	import CompletedTodayFooter from '$lib/components/view/CompletedTodayFooter.svelte';
	import CompleteOverdueDialog from '$lib/components/dialog/CompleteOverdueDialog.svelte';
	import { activeDayPart, groupByDayPart } from '$lib/utils/viewGroup';
	import { parseIso, dayKeyInTz, dayStartUtcInTz, isOverdue, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { calendarEventsOrEmpty } from '$lib/utils/calendar';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { toggleComplete, updateTaskFields } from '$lib/utils/taskActions';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartGroup } from '$lib/utils/viewGroup';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	let total = $state(0);
	let completedCount = $state(0);
	let calendarEvents = $state<CalendarEvent[]>([]);

	const list = useListMutator<Task>({
		onRemove: () => {
			total = Math.max(0, total - 1);
			completedCount += 1;
		}
	});

	// When a parent task is removed (completed), cascade-remove its subtasks so
	// they don't linger as orphaned roots in day-part sections.
	const baseMutator = list.mutator;
	const mutator = {
		replace: (t: Task) => baseMutator.replace(t),
		remove(id: number) {
			const toRemove: number[] = [];
			const collect = (parentId: number) => {
				for (const t of list.items) {
					if (t.parentId === parentId) {
						toRemove.push(t.id);
						collect(t.id);
					}
				}
			};
			collect(id);
			baseMutator.remove(id);
			for (const subId of toRemove) baseMutator.remove(subId);
		},
		insertAfter: (id: number, t: Task) => baseMutator.insertAfter(id, t),
		add: (t: Task) => baseMutator.add(t)
	};

	const tz = $derived(configStore.value?.timezone ?? null);
	const dayParts = $derived(configStore.value?.dayParts);

	const overdueTasks = $derived(list.items.filter((t) => isOverdue(t.dueAt, tz)));
	const todayTasks = $derived(list.items.filter((t) => !isOverdue(t.dueAt, tz)));
	const groups = $derived(groupByDayPart(todayTasks, dayParts));

	let now = $state(new Date());
	const active = $derived(activeDayPart(now, dayParts, tz));

	let completeDialogOpen = $state(false);
	let pendingCompleteTask = $state<Task | null>(null);

	const loader = usePageLoad(
		async (isValid) => {
			const ctxId = userStateStore.activeContextId ?? undefined;
			const todayKey = dayKeyInTz(new Date(), tz);
			const start = toIsoUtc(dayStartUtcInTz(todayKey, tz));
			const end = toIsoUtc(dayStartUtcInTz(shiftDayKey(todayKey, 1), tz));
			const [open, overdue, completed] = await Promise.all([
				viewsApi.today(getApiClient(), { contextId: ctxId }),
				viewsApi.overdue(getApiClient(), { contextId: ctxId }),
				viewsApi.completedToday(getApiClient(), { limit: 1, contextId: ctxId })
			]);
			if (!isValid()) return;
			const seen: Record<number, true> = {};
			const merged: Task[] = [];
			for (const t of [...overdue.items, ...open.items]) {
				if (seen[t.id]) continue;
				seen[t.id] = true;
				merged.push(t);
			}
			list.items = merged;
			total = open.total + overdue.total;
			completedCount = completed.total;
			void loadCalendarEvents(start, end, isValid);
		},
		{ errorMessage: $t('page.today.errorLoading'), autoLoad: false, initialLoading: true }
	);

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

	$effect(() => {
		function onVisible(): void {
			if (document.visibilityState !== 'visible') return;
			now = new Date();
			void loader.refetch();
		}
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	$effect(() => {
		const handler = (e: Event) => {
			const detail = (e as CustomEvent<{ task: Task }>).detail;
			const t = detail?.task;
			if (!t || t.status !== 'open' || !belongs(t)) return;
			const ctxId = userStateStore.activeContextId ?? null;
			if (ctxId !== null && t.contextId !== ctxId) return;
			if (list.items.some((x) => x.id === t.id)) return;
			list.items = [...list.items, t];
			total += 1;
		};
		window.addEventListener('turboist:task-created', handler);
		return () => window.removeEventListener('turboist:task-created', handler);
	});

	function isToday(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		return dayKeyInTz(dt, tz) === dayKeyInTz(new Date(), tz);
	}

	// Tasks belong on this page when they are due today OR are overdue.
	// Overdue tasks rendered above the day-part sections; rescheduling them to
	// today moves them into the day-part sections without a refetch.
	function belongs(t: Task): boolean {
		return isToday(t) || isOverdue(t.dueAt, tz);
	}

	function bulkMove(group: DayPartGroup, targetPart: DayPart): void {
		for (const task of group.tasks) {
			void updateTaskFields(task, mutator, { dayPart: targetPart });
		}
	}

	function onUncompletedFromFooter(): void {
		completedCount = Math.max(0, completedCount - 1);
	}

	async function handleToggle(task: Task): Promise<void> {
		if (task.status !== 'completed' && isOverdue(task.dueAt, tz)) {
			pendingCompleteTask = task;
			completeDialogOpen = true;
			return;
		}
		await toggleComplete(task, mutator, { belongs });
	}

	async function confirmOverdueComplete(completedAt: string): Promise<void> {
		const task = pendingCompleteTask;
		if (!task) return;
		await toggleComplete(task, mutator, { belongs, completedAt });
		pendingCompleteTask = null;
	}
</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0 && completedCount === 0 && calendarEvents.length === 0}
		emptyIcon={SunIcon}
		emptyTitle={$t('page.today.emptyTitle')}
		emptyDescription={$t('page.today.emptyDescription')}
	>
		<div class="flex flex-col gap-4 py-2">
			<CalendarEventList events={calendarEvents} timezone={tz} />

			{#if overdueTasks.length > 0}
				<section class="rounded-lg border border-destructive/40 px-1 py-2">
					<header class="flex items-center gap-2 px-2 py-1 text-sm font-medium text-destructive">
						<WarningIcon class="size-4" weight="fill" />
						<span>{$t('page.today.overdueTitle')}</span>
						<span class="text-muted-foreground">({overdueTasks.length})</span>
					</header>
					<TaskTree
						tasks={overdueTasks}
						hideTodayBadge
						showUnplannedBadge
						{mutator}
						{belongs}
						onToggle={handleToggle}
					/>
				</section>
			{/if}

			{#each groups as group (group.part)}
				<DayPartSection
					part={group.part}
					label={group.label}
					interval={group.interval}
					count={group.tasks.length}
					active={group.part === active || groups.length === 1}
					onBulkMove={(targetPart) => bulkMove(group, targetPart)}
				>
					<TaskTree
						tasks={group.tasks}
						hideTodayBadge
						showUnplannedBadge
						{mutator}
						{belongs}
						onToggle={handleToggle}
					/>
				</DayPartSection>
			{/each}

			<CompletedTodayFooter count={completedCount} onUncompleteOutside={onUncompletedFromFooter} />
		</div>
	</ViewContent>
</div>

<CompleteOverdueDialog
	bind:open={completeDialogOpen}
	task={pendingCompleteTask}
	onConfirm={confirmOverdueComplete}
/>
