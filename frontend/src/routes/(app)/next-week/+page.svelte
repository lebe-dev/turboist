<script lang="ts">
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import ArrowRightIcon from 'phosphor-svelte/lib/ArrowRight';
	import CalendarCheckIcon from 'phosphor-svelte/lib/CalendarCheck';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import { toast } from 'svelte-sonner';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskItem from '$lib/components/task/TaskItem.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import PlanSectionHeader from '$lib/components/view/PlanSectionHeader.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { formatDayKeyRange, nextWeekRangeKeys } from '$lib/utils/format';
	import { t, locale } from '$lib/i18n';

	const backlog = useListMutator<Task>();
	const week = useListMutator<Task>();

	const tz = $derived(configStore.value?.timezone ?? null);
	const nextRange = $derived(nextWeekRangeKeys(new Date(), tz));
	const nextRangeLabel = $derived(
		formatDayKeyRange(nextRange.startKey, nextRange.endKey, $locale, tz)
	);
	const headerSubtitle = $derived(
		$t('page.nextWeek.dateRangeLabel', { values: { range: nextRangeLabel } })
	);

	const weeklyLimit = $derived(configStore.value?.weekly.limit ?? null);
	const backlogLimit = $derived(configStore.value?.backlog.limit ?? null);
	// Counts come from planStatsStore (global), not from list lengths — list is
	// filtered by active context, so item counts can lag the limit enforced
	// server-side and let the user trigger a 422.
	const weekCount = $derived(planStatsStore.value?.week ?? week.items.length);
	const backlogCount = $derived(planStatsStore.value?.backlog ?? backlog.items.length);
	const weekFull = $derived(weeklyLimit !== null && weekCount >= weeklyLimit);
	const backlogFull = $derived(backlogLimit !== null && backlogCount >= backlogLimit);

	const loader = usePageLoad(
		async (isValid) => {
			const client = getApiClient();
			const ctx = userStateStore.activeContextId ?? undefined;
			const [backlogRes, weekRes] = await Promise.all([
				viewsApi.backlog(client, { contextId: ctx }),
				viewsApi.week(client, { contextId: ctx }),
				planStatsStore.load().catch(() => {})
			]);
			if (!isValid()) return;
			backlog.items = backlogRes.items;
			week.items = weekRes.items;
		},
		{ errorMessage: $t('page.nextWeek.errorLoading'), autoLoad: false, initialLoading: true }
	);

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});

	async function planForWeek(task: Task): Promise<void> {
		if (weekFull) {
			toast.error($t('page.nextWeek.weeklyLimitReached', { values: { weekCount, weeklyLimit: weeklyLimit ?? 0 } }));
			return;
		}
		try {
			const updated = await tasksApi.plan(getApiClient(), task.id, { state: 'week' });
			backlog.mutator.remove(task.id);
			week.items = [updated, ...week.items];
			void planStatsStore.load().catch(() => {});
		} catch (err) {
			toast.error(describeError(err, $t('page.nextWeek.failedPlan')));
		}
	}

	async function returnToBacklog(task: Task): Promise<void> {
		if (backlogFull) {
			toast.error($t('page.nextWeek.backlogLimitReached', { values: { backlogCount, backlogLimit: backlogLimit ?? 0 } }));
			return;
		}
		try {
			const updated = await tasksApi.plan(getApiClient(), task.id, { state: 'backlog' });
			week.mutator.remove(task.id);
			backlog.items = [updated, ...backlog.items];
			void planStatsStore.load().catch(() => {});
		} catch (err) {
			toast.error(describeError(err, $t('page.nextWeek.failedMove')));
		}
	}
</script>

<ViewHeader subtitle={headerSubtitle} />

<div class="grid grid-cols-1 gap-4 px-2 py-3 sm:grid-cols-2 sm:px-4">
	<section class="flex flex-col rounded-md border border-border/60 bg-background">
		<PlanSectionHeader
			title={$t('page.nextWeek.backlogTitle')}
			count={backlogLimit !== null ? backlogCount : backlog.items.length}
			limit={backlogLimit}
		/>
		<div class="min-h-[200px]">
			<ViewContent
				loading={loader.loading}
				isEmpty={backlog.items.length === 0}
				emptyIcon={StackIcon}
				emptyTitle={$t('page.nextWeek.backlogEmptyTitle')}
				emptyDescription={$t('page.nextWeek.backlogEmptyDesc')}
			>
				<div class="flex flex-col">
					{#each backlog.items as task (task.id)}
						<div class="flex items-stretch border-b border-border/40 last:border-b-0">
							<div class="min-w-0 flex-1">
								<TaskItem
									{task}
									mutator={backlog.mutator}
									belongs={(t) => t.planState === 'backlog'}
									onToggle={(t) => toggleComplete(t, backlog.mutator)}
								/>
							</div>
							<button
								type="button"
								onclick={() => void planForWeek(task)}
								disabled={weekFull}
								aria-label={$t('page.nextWeek.planForWeek')}
								title={weekFull
									? $t('page.nextWeek.weeklyLimitReachedShort', { values: { limit: weeklyLimit ?? 0 } })
									: $t('page.nextWeek.planForWeek')}
								class="flex w-10 shrink-0 items-center justify-center text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
							>
								<ArrowRightIcon class="size-4" weight="bold" />
							</button>
						</div>
					{/each}
				</div>
			</ViewContent>
		</div>
	</section>

	<section class="flex flex-col rounded-md border border-border/60 bg-background">
		<PlanSectionHeader
			title={$t('page.nextWeek.nextWeekTitle')}
			count={weeklyLimit !== null ? weekCount : null}
			limit={weeklyLimit}
		/>
		<div class="min-h-[200px]">
			<ViewContent
				loading={loader.loading}
				isEmpty={week.items.length === 0}
				emptyIcon={CalendarCheckIcon}
				emptyTitle={$t('page.nextWeek.weekEmptyTitle')}
				emptyDescription={$t('page.nextWeek.weekEmptyDesc')}
			>
				<div class="flex flex-col">
					{#each week.items as task (task.id)}
						<div class="flex items-stretch border-b border-border/40 last:border-b-0">
							<button
								type="button"
								onclick={() => void returnToBacklog(task)}
								disabled={backlogFull}
								aria-label={$t('page.nextWeek.returnToBacklog')}
								title={backlogFull
									? $t('page.nextWeek.backlogLimitReachedShort', { values: { limit: backlogLimit ?? 0 } })
									: $t('page.nextWeek.returnToBacklog')}
								class="flex w-10 shrink-0 items-center justify-center text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
							>
								<ArrowLeftIcon class="size-4" weight="bold" />
							</button>
							<div class="min-w-0 flex-1">
								<TaskItem
									{task}
									mutator={week.mutator}
									belongs={(t) => t.planState === 'week'}
									onToggle={(t) => toggleComplete(t, week.mutator)}
								/>
							</div>
						</div>
					{/each}
				</div>
			</ViewContent>
		</div>
	</section>
</div>
