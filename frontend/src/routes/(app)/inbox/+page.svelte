<script lang="ts">
	import { onMount } from 'svelte';
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
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let count = $state(0);
	let warn = $state(false);
	let loading = $state(true);
	let quickOpen = $state(false);

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			count = Math.max(0, count - 1);
		}
	};

	async function load(): Promise<void> {
		loading = true;
		try {
			const res = await tasksApi.inbox(getApiClient());
			items = res.tasks;
			count = res.count;
			warn = res.warnThresholdExceeded;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load inbox'));
		} finally {
			loading = false;
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
			items = [...items, created];
			count = count + 1;
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

	onMount(load);
</script>

<ViewHeader
	title="Inbox"
	subtitle={loading ? 'Loading…' : `${count} task${count === 1 ? '' : 's'}`}
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
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0}
		<EmptyState
			icon={InboxIcon}
			title="Inbox is empty"
			description="Tasks captured without a project land here. Press Q to add one."
		/>
	{:else}
		<TaskTree
			tasks={items}
			showProject={false}
			onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
			onPinToggle={(t) => togglePin(t, mutator)}
			onDelete={(t) => deleteTask(t, mutator)}
		/>
	{/if}
</div>

<QuickAddDialog bind:open={quickOpen} onSubmit={onQuickSubmit} />
