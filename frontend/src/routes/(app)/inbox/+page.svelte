<script lang="ts">
	import { toast } from 'svelte-sonner';
	import InboxIcon from 'phosphor-svelte/lib/Tray';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import { Button } from '$lib/components/ui/button';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import type { Task, TaskInput } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	let count = $state(0);
	let warn = $state(false);
	let quickOpen = $state(false);

	const list = useListMutator<Task>({ onRemove: () => { count = Math.max(0, count - 1); } });
	const { mutator } = list;

	const loader = usePageLoad(async () => {
		const res = await tasksApi.inbox(getApiClient());
		list.items = res.tasks;
		count = res.count;
		warn = res.warnThresholdExceeded;
	}, { errorMessage: 'Failed to load inbox' });

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
			count = count + 1;
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

</script>

<ViewHeader
	title="Inbox"
	subtitle={loader.loading ? 'Loading…' : `${count} task${count === 1 ? '' : 's'}`}
>
	{#snippet actions()}
		<Button size="sm" onclick={() => (quickOpen = true)} class="gap-2">
			<PlusIcon class="size-4" />
			Add task
		</Button>
	{/snippet}
	{#snippet banner()}
		{#if warn && configStore.value}
			<div
				class="flex items-start gap-2 rounded border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
			>
				<WarningIcon class="size-4 shrink-0" />
				<span>
					Inbox is over the warn threshold ({configStore.value.inbox.warnThreshold}). Process or
					move tasks out.
				</span>
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
