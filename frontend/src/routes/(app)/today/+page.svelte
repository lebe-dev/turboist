<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import TaskEditorSheet from '$lib/components/task/TaskEditorSheet.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import { groupByDayPart } from '$lib/utils/viewGroup';
	import { parseIso, dayKeyInTz } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		saveEdit,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let editing = $state<Task | null>(null);
	let editorOpen = $state(false);

	const groups = $derived(groupByDayPart(items));

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
			const res = await viewsApi.today(getApiClient());
			items = res.items;
			total = res.total;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load today'));
		} finally {
			loading = false;
		}
	}

	function isToday(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		const tz = configStore.value?.timezone ?? null;
		return dayKeyInTz(dt, tz) === dayKeyInTz(new Date(), tz);
	}

	function openEditor(task: Task): void {
		editing = task;
		editorOpen = true;
	}

	onMount(load);
</script>

<ViewHeader title="Today" subtitle={loading ? 'Loading…' : `${total} task${total === 1 ? '' : 's'}`} />

<div class="px-2 py-2">
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0}
		<EmptyState
			icon={SunIcon}
			title="Nothing for today"
			description="No tasks are scheduled for today. Enjoy the calm."
		/>
	{:else}
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.part)}
				<section>
					<h2 class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
						{group.label}
					</h2>
					<TaskTree
						tasks={group.tasks}
						onToggle={(t) => toggleComplete(t, mutator, { belongs: isToday })}
						onPinToggle={(t) => togglePin(t, mutator)}
						onDelete={(t) => deleteTask(t, mutator)}
						onEdit={openEditor}
					/>
				</section>
			{/each}
		</div>
	{/if}
</div>

<TaskEditorSheet
	bind:open={editorOpen}
	task={editing}
	onSubmit={(id, payload) => saveEdit(id, payload, mutator, isToday)}
/>
