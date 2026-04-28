<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';

	let {
		tasks,
		showProject = true,
		onToggle,
		onEdit,
		onDelete,
		onPinToggle
	}: {
		tasks: Task[];
		showProject?: boolean;
		onToggle?: (task: Task) => void;
		onEdit?: (task: Task) => void;
		onDelete?: (task: Task) => void;
		onPinToggle?: (task: Task) => void;
	} = $props();
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each tasks as task (task.id)}
		<TaskItem {task} {showProject} {onToggle} {onEdit} {onDelete} {onPinToggle} />
	{/each}
</div>
