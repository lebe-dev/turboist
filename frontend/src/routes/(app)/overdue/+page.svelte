<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import { Button } from '$lib/components/ui/button';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskItem from '$lib/components/task/TaskItem.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import { toIsoUtc, dayKeyInTz, dayStartUtcInTz, shiftDayKey } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let total = $state(0);
	let loading = $state(true);

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			total = Math.max(0, total - 1);
		}
	};

	async function load(): Promise<void> {
		loading = true;
		try {
			const res = await viewsApi.overdue(getApiClient());
			items = res.items;
			total = res.total;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load overdue'));
		} finally {
			loading = false;
		}
	}

	function startOfDayUtc(offsetDays: number): string {
		const tz = configStore.value?.timezone ?? null;
		const todayKey = dayKeyInTz(new Date(), tz);
		const targetKey = offsetDays === 0 ? todayKey : shiftDayKey(todayKey, offsetDays);
		return toIsoUtc(dayStartUtcInTz(targetKey, tz));
	}

	async function moveToDay(task: Task, offsetDays: number, label: string): Promise<void> {
		try {
			const updated = await tasksApi.update(getApiClient(), task.id, {
				dueAt: startOfDayUtc(offsetDays),
				dueHasTime: false
			});
			mutator.remove(task.id);
			toast.success(`Moved to ${label}`);
			void updated;
		} catch (err) {
			toast.error(describeError(err, `Failed to move to ${label}`));
		}
	}

	async function moveToBacklog(task: Task): Promise<void> {
		try {
			await tasksApi.plan(getApiClient(), task.id, { state: 'backlog' });
			mutator.remove(task.id);
			toast.success('Moved to backlog');
		} catch (err) {
			toast.error(describeError(err, 'Failed to move to backlog'));
		}
	}

	onMount(load);
</script>

<ViewHeader
	title="Overdue"
	subtitle={loading ? 'Loading…' : `${total} task${total === 1 ? '' : 's'} past due`}
/>

<div class="px-2 py-2">
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0}
		<EmptyState
			icon={WarningIcon}
			title="No overdue tasks"
			description="You're all caught up."
		/>
	{:else}
		<div class="flex flex-col">
			{#each items as task (task.id)}
				<div class="border-b border-border/50">
					<TaskItem
						{task}
						onToggle={(t) => toggleComplete(t, mutator)}
						onPinToggle={(t) => togglePin(t, mutator)}
						onDelete={(t) => deleteTask(t, mutator)}
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
	{/if}
</div>
