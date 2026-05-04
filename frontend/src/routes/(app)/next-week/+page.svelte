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
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const backlog = useListMutator<Task>();
	const week = useListMutator<Task>();

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
		{ errorMessage: 'Failed to load planning view', autoLoad: false, initialLoading: true }
	);

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});

	async function planForWeek(task: Task): Promise<void> {
		if (weekFull) {
			toast.error(`Weekly limit reached (${weekCount}/${weeklyLimit})`);
			return;
		}
		try {
			const updated = await tasksApi.plan(getApiClient(), task.id, { state: 'week' });
			backlog.mutator.remove(task.id);
			week.items = [updated, ...week.items];
			void planStatsStore.load().catch(() => {});
		} catch (err) {
			toast.error(describeError(err, 'Failed to plan task'));
		}
	}

	async function returnToBacklog(task: Task): Promise<void> {
		if (backlogFull) {
			toast.error(`Backlog limit reached (${backlogCount}/${backlogLimit})`);
			return;
		}
		try {
			const updated = await tasksApi.plan(getApiClient(), task.id, { state: 'backlog' });
			week.mutator.remove(task.id);
			backlog.items = [updated, ...backlog.items];
			void planStatsStore.load().catch(() => {});
		} catch (err) {
			toast.error(describeError(err, 'Failed to move to backlog'));
		}
	}
</script>

<div class="grid grid-cols-1 gap-4 px-2 py-3 sm:grid-cols-2 sm:px-4">
	<section class="flex flex-col rounded-md border border-border/60 bg-background">
		<header
			class="flex items-center justify-between gap-2 border-b border-border/50 px-3 py-2"
		>
			<h2
				class="text-sm font-semibold uppercase tracking-wide"
				class:text-muted-foreground={!backlogFull}
				class:text-red-600={backlogFull}
				class:dark:text-red-400={backlogFull}
			>
				Backlog
			</h2>
			{#if backlogLimit !== null}
				<span
					class="font-mono text-[11px] tabular-nums"
					class:text-muted-foreground={!backlogFull}
					class:text-red-600={backlogFull}
					class:dark:text-red-400={backlogFull}
					class:font-semibold={backlogFull}
				>
					{backlogCount} / {backlogLimit}
				</span>
			{:else}
				<span class="font-mono text-[11px] tabular-nums text-muted-foreground">
					{backlog.items.length}
				</span>
			{/if}
		</header>
		<div class="min-h-[200px]">
			<ViewContent
				loading={loader.loading}
				isEmpty={backlog.items.length === 0}
				emptyIcon={StackIcon}
				emptyTitle="Backlog is empty"
				emptyDescription="Park tasks here when they're not ready yet."
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
								aria-label="Plan for next week"
								title={weekFull
									? `Weekly limit reached (${weeklyLimit})`
									: 'Plan for next week'}
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
		<header
			class="flex items-center justify-between gap-2 border-b border-border/50 px-3 py-2"
		>
			<h2
				class="text-sm font-semibold uppercase tracking-wide"
				class:text-muted-foreground={!weekFull}
				class:text-red-600={weekFull}
				class:dark:text-red-400={weekFull}
			>
				Next week
			</h2>
			{#if weeklyLimit !== null}
				<span
					class="font-mono text-[11px] tabular-nums"
					class:text-muted-foreground={!weekFull}
					class:text-red-600={weekFull}
					class:dark:text-red-400={weekFull}
					class:font-semibold={weekFull}
				>
					{weekCount} / {weeklyLimit}
				</span>
			{/if}
		</header>
		<div class="min-h-[200px]">
			<ViewContent
				loading={loader.loading}
				isEmpty={week.items.length === 0}
				emptyIcon={CalendarCheckIcon}
				emptyTitle="Nothing planned"
				emptyDescription="Move tasks here from the backlog with the arrow."
			>
				<div class="flex flex-col">
					{#each week.items as task (task.id)}
						<div class="flex items-stretch border-b border-border/40 last:border-b-0">
							<button
								type="button"
								onclick={() => void returnToBacklog(task)}
								disabled={backlogFull}
								aria-label="Return to backlog"
								title={backlogFull
									? `Backlog limit reached (${backlogLimit})`
									: 'Return to backlog'}
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
