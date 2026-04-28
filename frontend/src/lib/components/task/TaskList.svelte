<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';

	let {
		tasks,
		showProject = true,
		hideDayPart = false,
		onToggle,
		onDelete,
		onPinToggle
	}: {
		tasks: Task[];
		showProject?: boolean;
		hideDayPart?: boolean;
		onToggle?: (task: Task) => void;
		onDelete?: (task: Task) => void;
		onPinToggle?: (task: Task) => void;
	} = $props();
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each tasks as task (task.id)}
		<TaskItem {task} {showProject} {hideDayPart} {onToggle} {onDelete} {onPinToggle} />
	{/each}
</div>
