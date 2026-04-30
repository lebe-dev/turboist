<script lang="ts">
	import { toast } from 'svelte-sonner';
	import InboxIcon from 'phosphor-svelte/lib/Tray';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import BroomIcon from 'phosphor-svelte/lib/Broom';
	import { Button } from '$lib/components/ui/button';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { inboxStatsStore } from '$lib/stores/inboxStats.svelte';
	import type { Task, TaskInput } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { dayKeyInTz, dayStartUtcInTz, toIsoUtc } from '$lib/utils/format';


	let quickOpen = $state(false);
	let creatingOverflow = $state(false);

	const warnThreshold = $derived(configStore.value?.inbox.warnThreshold ?? 0);
	const overflowTask = $derived(configStore.value?.inbox.overflowTask ?? null);

	function applyCount(count: number): void {
		inboxStatsStore.set(count, warnThreshold > 0 && count >= warnThreshold);
	}

	const list = useListMutator<Task>({
		onRemove: () => applyCount(Math.max(0, inboxStatsStore.count - 1))
	});
	const { mutator } = list;

	const loader = usePageLoad(async () => {
		const res = await tasksApi.inbox(getApiClient());
		list.items = res.tasks;
		inboxStatsStore.set(res.count, res.warnThresholdExceeded);
	}, { errorMessage: 'Failed to load inbox' });

	async function createOverflowTask(): Promise<void> {
		if (!overflowTask || creatingOverflow) return;
		creatingOverflow = true;
		try {
			const tz = configStore.value?.timezone ?? null;
			const todayKey = dayKeyInTz(new Date(), tz);
			const payload: TaskInput = {
				title: overflowTask.title,
				priority: overflowTask.priority,
				dueAt: toIsoUtc(dayStartUtcInTz(todayKey, tz)),
				dueHasTime: false
			};
			const created = await tasksApi.createInbox(getApiClient(), payload);
			list.items = [...list.items, created];
			applyCount(inboxStatsStore.count + 1);
			toast.success('Cleanup task created for today');
		} catch (err) {
			toast.error(describeError(err, 'Failed to create cleanup task'));
		} finally {
			creatingOverflow = false;
		}
	}

	async function onQuickSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		try {
			const client = getApiClient();
			if (target.projectId !== null) {
				await projectsApi.createTask(client, target.projectId, payload);
				toast.success('Task added to project');
				return;
			}
			const created = await tasksApi.createInbox(client, payload);
			list.items = [...list.items, created];
			applyCount(inboxStatsStore.count + 1);
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

</script>

<ViewHeader>
	{#snippet actions()}
		<Button size="sm" onclick={() => (quickOpen = true)} class="gap-2">
			<PlusIcon class="size-4" />
			Add task
		</Button>
	{/snippet}
	{#snippet banner()}
		{#if inboxStatsStore.warnThresholdExceeded && configStore.value}
			<div
				class="flex flex-col gap-2 rounded border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400 sm:flex-row sm:items-center sm:justify-between"
			>
				<div class="flex items-start gap-2">
					<WarningIcon class="size-4 shrink-0" />
					<span>
						Inbox is over the warn threshold ({configStore.value.inbox.warnThreshold}). Schedule a
						cleanup task for today to work it down.
					</span>
				</div>
				{#if overflowTask}
					<Button
						size="sm"
						variant="outline"
						class="shrink-0 gap-2 border-amber-500/50 bg-background/60 text-amber-800 hover:bg-amber-500/10 dark:text-amber-300"
						disabled={creatingOverflow}
						onclick={createOverflowTask}
					>
						<BroomIcon class="size-4" />
						{overflowTask.title}
					</Button>
				{/if}
			</div>
		{/if}
	{/snippet}
</ViewHeader>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={InboxIcon}
		emptyTitle="Inbox is empty"
		emptyDescription="Tasks captured without a project land here. Press Q to add one."
	>
		<TaskTree
			tasks={list.items}
			showProject={false}
			{mutator}
			belongs={(t) => t.inboxId !== null}
			onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
		/>
	</ViewContent>
</div>

<QuickAddDialog bind:open={quickOpen} onSubmit={onQuickSubmit} />
