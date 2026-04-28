<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import LimitBadge from '$lib/components/view/LimitBadge.svelte';
	import { groupByDay } from '$lib/utils/viewGroup';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let total = $state(0);
	let loading = $state(true);

	const groups = $derived(groupByDay(items, configStore.value?.timezone ?? null));
	const limit = $derived(configStore.value?.weekly.limit ?? null);
	const exceeded = $derived(limit !== null && total >= limit);

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
			const res = await viewsApi.week(getApiClient());
			items = res.items;
			total = res.total;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load week'));
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<ViewHeader title="This week" subtitle={loading ? 'Loading…' : 'Tasks planned for the week'}>
	{#snippet actions()}
		{#if limit !== null}
			<LimitBadge count={total} {limit} />
		{/if}
	{/snippet}
	{#snippet banner()}
		{#if exceeded && limit !== null}
			<div
				class="rounded border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
			>
				Weekly limit reached ({total}/{limit}). Adding more tasks to the week will be rejected.
			</div>
		{/if}
	{/snippet}
</ViewHeader>

<div class="px-2 py-2">
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0}
		<EmptyState
			icon={CalendarIcon}
			title="Week is empty"
			description="Plan tasks for this week from Backlog or by setting a due date."
		/>
	{:else}
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.dayKey)}
				<section>
					<h2 class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
						{group.label}
					</h2>
					<TaskTree
						tasks={group.tasks}
						onToggle={(t) => toggleComplete(t, mutator)}
						onPinToggle={(t) => togglePin(t, mutator)}
						onDelete={(t) => deleteTask(t, mutator)}
					/>
				</section>
			{/each}
		</div>
	{/if}
</div>
