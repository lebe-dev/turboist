<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import { buildTree } from '$lib/utils/taskTree';
	import type { ListMutator } from '$lib/utils/taskActions';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';

	let {
		tasks,
		draggable = true,
		mutator,
		belongs,
		onToggle,
		showProject = false
	}: {
		tasks: Task[];
		draggable?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		showProject?: boolean;
	} = $props();

	let open = $state(false);
	const rootCount = $derived(buildTree(tasks).length);
</script>

<div class="border-t border-border/40">
	<button
		type="button"
		class="flex w-full items-center gap-2 px-3 py-2 text-left text-xs font-medium text-muted-foreground hover:text-foreground"
		onclick={() => (open = !open)}
		aria-expanded={open}
	>
		{#if open}
			<CaretDownIcon class="size-3" />
		{:else}
			<CaretRightIcon class="size-3" />
		{/if}
		<CheckCircleIcon class="size-3.5" weight="fill" />
		<span>Completed</span>
		<span class="text-muted-foreground/70">{rootCount}</span>
	</button>
	{#if open}
		<TaskTree
			{tasks}
			{showProject}
			{draggable}
			{mutator}
			{belongs}
			{onToggle}
		/>
	{/if}
</div>
