<script lang="ts">
	import { toast } from 'svelte-sonner';
	import InboxIcon from 'phosphor-svelte/lib/Tray';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { inboxStatsStore } from '$lib/stores/inboxStats.svelte';
	import type { Task, TaskInput } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { dayKeyInTz, dayStartUtcInTz, toIsoUtc } from '$lib/utils/format';


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

	const sortedTasks = $derived(
		[...list.items].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
	);

	const loader = usePageLoad(async () => {
		const res = await tasksApi.inbox(getApiClient());
		list.items = res.tasks;
		inboxStatsStore.set(res.count, res.warnThresholdExceeded);
	}, { errorMessage: 'Failed to load inbox' });

	$effect(() => {
		const handler = (e: Event) => {
			const detail = (e as CustomEvent<{ task: Task; projectId: number | null }>).detail;
			if (!detail || detail.projectId !== null) return;
			list.items = [...list.items, detail.task];
			applyCount(inboxStatsStore.count + 1);
		};
		window.addEventListener('turboist:task-created', handler);
		return () => window.removeEventListener('turboist:task-created', handler);
	});

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

</script>

<div class="px-2 py-2">
	{#if inboxStatsStore.warnThresholdExceeded && configStore.value}
		<p class="mt-3 mb-4 px-3 text-sm text-muted-foreground">
			Inbox is over capacity ({inboxStatsStore.count}/{configStore.value.inbox.warnThreshold}).{#if overflowTask}
				 <button
					type="button"
					class="underline underline-offset-2 hover:text-foreground disabled:opacity-50"
					disabled={creatingOverflow}
					onclick={createOverflowTask}
				>Create «{overflowTask.title}» task for today</button>.
			{/if}
		</p>
	{/if}
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={InboxIcon}
		emptyTitle="Inbox is empty"
		emptyDescription="Tasks captured without a project land here. Press Q to add one."
	>
		<TaskTree
			tasks={sortedTasks}
			showProject={false}
			{mutator}
			belongs={(t) => t.inboxId !== null}
			onToggle={(t) => toggleComplete(t, mutator)}
		/>
	</ViewContent>
</div>
