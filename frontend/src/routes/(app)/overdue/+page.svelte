<script lang="ts">
	import { toast } from 'svelte-sonner';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import { Button } from '$lib/components/ui/button';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskItem from '$lib/components/task/TaskItem.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { toIsoUtc, dayKeyInTz, dayStartUtcInTz, shiftDayKey, isOverdue } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';


	let total = $state(0);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const loader = usePageLoad(async (isValid) => {
		const res = await viewsApi.overdue(getApiClient(), {
			contextId: userStateStore.activeContextId ?? undefined
		});
		if (!isValid()) return;
		list.items = res.items;
		total = res.total;
	}, { errorMessage: 'Failed to load overdue', autoLoad: false, initialLoading: true });

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});

	function startOfDayUtc(offsetDays: number): string {
		const tz = configStore.value?.timezone ?? null;
		const todayKey = dayKeyInTz(new Date(), tz);
		const targetKey = offsetDays === 0 ? todayKey : shiftDayKey(todayKey, offsetDays);
		return toIsoUtc(dayStartUtcInTz(targetKey, tz));
	}

	async function moveToDay(task: Task, offsetDays: number, label: string): Promise<void> {
		try {
			await tasksApi.update(getApiClient(), task.id, {
				dueAt: startOfDayUtc(offsetDays),
				dueHasTime: false
			});
			mutator.remove(task.id);
			toast.success(`Moved to ${label}`);
		} catch (err) {
			toast.error(describeError(err, `Failed to move to ${label}`));
		}
	}

	async function moveToBacklog(task: Task): Promise<void> {
		const client = getApiClient();
		try {
			if (task.dueAt) {
				await tasksApi.update(client, task.id, { dueAt: null, dueHasTime: false });
			}
			await tasksApi.plan(client, task.id, { state: 'backlog' });
			mutator.remove(task.id);
			toast.success('Moved to backlog');
			void planStatsStore.load().catch(() => {});
		} catch (err) {
			toast.error(describeError(err, 'Failed to move to backlog'));
		}
	}

</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={WarningIcon}
		emptyTitle="No overdue tasks"
		emptyDescription="You're all caught up."
	>
		<div class="flex flex-col">
			{#each list.items as task (task.id)}
				<div class="border-b border-border/50">
					<TaskItem
						{task}
						{mutator}
						belongs={(t) => isOverdue(t.dueAt, configStore.value?.timezone ?? null)}
						onToggle={(t) => toggleComplete(t, mutator)}
					/>
					<div class="flex flex-wrap gap-2 px-4 pb-2 pl-10">
						<Button size="sm" variant="outline" onclick={() => moveToDay(task, 0, 'today')}>
							Today
						</Button>
						<Button size="sm" variant="outline" onclick={() => moveToDay(task, 1, 'tomorrow')}>
							Tomorrow
						</Button>
						<Button size="sm" variant="outline" onclick={() => moveToBacklog(task)}>
							Backlog
						</Button>
					</div>
				</div>
			{/each}
		</div>
	</ViewContent>
</div>
