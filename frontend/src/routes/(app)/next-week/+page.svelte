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
	import { buildTree } from '$lib/utils/taskTree';
	import TaskItem from '$lib/components/task/TaskItem.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import PlanSectionHeader from '$lib/components/view/PlanSectionHeader.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { refreshSidebarBundle } from '$lib/realtime/refresh';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { untrack } from 'svelte';
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

	const backlogTree = $derived(buildTree(backlog.items));
	const weekTree = $derived(buildTree(week.items));

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
				// The plan counters are shown as a cap ("3 of 5"), so they must be
				// server truth rather than list lengths. They now ride in the sidebar
				// bundle — the standalone /stats/plan endpoint was a strict subset.
				refreshSidebarBundle().catch(() => {})
			]);
			if (!isValid()) return;
			backlog.setFromServer(backlogRes.items);
			// The week view also returns tasks that merely have a due date inside the
			// current week; here we only want tasks explicitly planned for the week.
			week.setFromServer(weekRes.items.filter((t) => t.planState === 'week'));
		},
		{
			errorMessage: $t('page.nextWeek.errorLoading'),
			autoLoad: false,
			initialLoading: true,
			// Two lists, one loader: either mutation must invalidate an in-flight
			// revalidation, since the fetcher rewrites both.
			epoch: () => backlog.epoch + week.epoch
		}
	);

	// See today/+page.svelte: $derived gates equal values, untrack keeps the
	// fetcher's own store reads out of this effect's dependency set.
	const activeContextId = $derived(userStateStore.activeContextId);
	$effect(() => {
		void activeContextId;
		untrack(() => void loader.refetch());
	});

	useInvalidation(['tasks', 'plan'], () => void loader.revalidate());

	async function planForWeek(task: Task): Promise<void> {
		if (weekFull) {
			toast.error($t('page.nextWeek.weeklyLimitReached', { values: { weekCount, weeklyLimit: weeklyLimit ?? 0 } }));
			return;
		}
		try {
			const updated = await tasksApi.plan(getApiClient(), task.id, { state: 'week' });
			backlog.mutator.remove(task.id);
			week.items = [updated, ...week.items];
			void refreshSidebarBundle().catch(() => {});
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
			void refreshSidebarBundle().catch(() => {});
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
			total={backlog.items.length}
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
					{#each backlogTree as node (node.task.id)}
						<div class="border-b border-border/40 last:border-b-0">
							<div class="flex items-stretch">
								<div class="min-w-0 flex-1">
									<TaskItem
										task={node.task}
										mutator={backlog.mutator}
										belongs={(t) => t.planState === 'backlog'}
										onToggle={(t) => toggleComplete(t, backlog.mutator)}
									/>
								</div>
								<button
									type="button"
									onclick={() => void planForWeek(node.task)}
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
							{#each node.children as child (child.task.id)}
								<div class="flex items-stretch border-t border-border/30">
									<div class="min-w-0 flex-1">
										<TaskItem
											task={child.task}
											depth={1}
											mutator={backlog.mutator}
											belongs={(t) => t.planState === 'backlog'}
											onToggle={(t) => toggleComplete(t, backlog.mutator)}
										/>
									</div>
									<div class="w-10 shrink-0"></div>
								</div>
							{/each}
						</div>
					{/each}
				</div>
			</ViewContent>
		</div>
	</section>

	<section class="flex flex-col rounded-md border border-border/60 bg-background">
		<PlanSectionHeader
			title={$t('page.nextWeek.nextWeekTitle')}
			count={weeklyLimit !== null ? weekCount : week.items.length}
			limit={weeklyLimit}
			total={week.items.length}
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
					{#each weekTree as node (node.task.id)}
						<div class="border-b border-border/40 last:border-b-0">
							<div class="flex items-stretch">
								<button
									type="button"
									onclick={() => void returnToBacklog(node.task)}
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
										task={node.task}
										mutator={week.mutator}
										belongs={(t) => t.planState === 'week'}
										onToggle={(t) => toggleComplete(t, week.mutator)}
									/>
								</div>
							</div>
							{#each node.children as child (child.task.id)}
								<div class="flex items-stretch border-t border-border/30">
									<div class="w-10 shrink-0"></div>
									<div class="min-w-0 flex-1">
										<TaskItem
											task={child.task}
											depth={1}
											mutator={week.mutator}
											belongs={(t) => t.planState === 'week'}
											onToggle={(t) => toggleComplete(t, week.mutator)}
										/>
									</div>
								</div>
							{/each}
						</div>
					{/each}
				</div>
			</ViewContent>
		</div>
	</section>
</div>
