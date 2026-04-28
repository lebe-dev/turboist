<script lang="ts">
	import type { Task } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import TaskItem from './TaskItem.svelte';

	let {
		tasks,
		showProject = true,
		hideDayPart = false,
		mutator,
		belongs,
		onToggle
	}: {
		tasks: Task[];
		showProject?: boolean;
		hideDayPart?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
	} = $props();
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each tasks as task (task.id)}
		<TaskItem {task} {showProject} {hideDayPart} {mutator} {belongs} {onToggle} />
	{/each}
</div>
