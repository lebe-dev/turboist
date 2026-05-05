<script lang="ts">
	import { toast } from 'svelte-sonner';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import { describeError, toggleComplete } from '$lib/utils/taskActions';

	let {
		count,
		onUncompleteOutside
	}: {
		count: number;
		onUncompleteOutside?: (task: Task) => void;
	} = $props();

	let expanded = $state(false);
	let loaded = $state(false);
	let loading = $state(false);
	let items = $state<Task[]>([]);

	async function ensureLoaded(): Promise<void> {
		if (loaded || loading) return;
		loading = true;
		try {
			const res = await viewsApi.completedToday(getApiClient());
			items = res.items;
			loaded = true;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load completed tasks'));
		} finally {
			loading = false;
		}
	}

	async function toggle(): Promise<void> {
		expanded = !expanded;
		if (expanded) await ensureLoaded();
	}

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			onUncompleteOutside?.({ id } as Task);
		}
	};

	async function onItemToggle(task: Task): Promise<void> {
		// Inside this list every item is completed; toggling means uncompleting.
		// Removing it from this list and bubbling up so the parent can refresh
		// the open list / counters.
		await toggleComplete(task, mutator, { belongs: () => false });
	}
</script>

{#if count > 0}
	<div class="flex flex-col items-stretch gap-2 pt-6">
		<button
			type="button"
			class="mx-auto inline-flex items-center gap-2 rounded-md px-3 py-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground/70 transition-colors hover:bg-accent hover:text-foreground"
			onclick={toggle}
			aria-expanded={expanded}
		>
			<span>Completed today</span>
			<span
				class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-muted px-1.5 text-[11px] font-medium text-muted-foreground"
			>
				{count}
			</span>
			<CaretDownIcon
				class="size-3.5 transition-transform {expanded ? 'rotate-180' : ''}"
			/>
		</button>

		{#if expanded}
			<div class="px-1">
				{#if loading}
					<div class="px-4 py-4 text-sm text-muted-foreground">Loading…</div>
				{:else if items.length === 0}
					<div class="px-4 py-4 text-sm text-muted-foreground">No tasks completed today.</div>
				{:else}
					<TaskTree tasks={items} hideDue onToggle={onItemToggle} />
				{/if}
			</div>
		{/if}
	</div>
{/if}
