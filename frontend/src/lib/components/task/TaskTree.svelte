<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { buildTree, type TaskNode } from '$lib/utils/taskTree';
	import TaskItem from './TaskItem.svelte';
	import Self from './TaskTree.svelte';

	let {
		tasks,
		nodes,
		depth = 0,
		showProject = true,
		onToggle,
		onEdit,
		onDelete,
		onPinToggle
	}: {
		tasks?: Task[];
		nodes?: TaskNode[];
		depth?: number;
		showProject?: boolean;
		onToggle?: (task: Task) => void;
		onEdit?: (task: Task) => void;
		onDelete?: (task: Task) => void;
		onPinToggle?: (task: Task) => void;
	} = $props();

	const resolved = $derived<TaskNode[]>(nodes ?? (tasks ? buildTree(tasks) : []));
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each resolved as node (node.task.id)}
		<TaskItem
			task={node.task}
			{depth}
			{showProject}
			{onToggle}
			{onEdit}
			{onDelete}
			{onPinToggle}
		/>
		{#if node.children.length > 0}
			<Self
				nodes={node.children}
				depth={depth + 1}
				{showProject}
				{onToggle}
				{onEdit}
				{onDelete}
				{onPinToggle}
			/>
		{/if}
	{/each}
</div>
