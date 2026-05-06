<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { buildTree, type TaskNode } from '$lib/utils/taskTree';
	import type { ListMutator } from '$lib/utils/taskActions';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import {
		buildProjectsById,
		buildTasksById,
		isTaskVisible
	} from '$lib/utils/visibility';
	import TaskItem from './TaskItem.svelte';
	import Self from './TaskTree.svelte';

	let {
		tasks,
		nodes,
		depth = 0,
		showProject = true,
		hideTodayBadge = false,
		hideTomorrowBadge = false,
		hideDue = false,
		draggable = false,
		showUnplannedBadge = false,
		mutator,
		belongs,
		onToggle
	}: {
		tasks?: Task[];
		nodes?: TaskNode[];
		depth?: number;
		showProject?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
		hideDue?: boolean;
		draggable?: boolean;
		showUnplannedBadge?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
	} = $props();

	const visibleTasks = $derived.by(() => {
		if (!tasks) return undefined;
		if (!settingsStore.publicView) return tasks;
		const tasksById = buildTasksById(tasks);
		const projectsById = buildProjectsById(projectsStore.items ?? []);
		return tasks.filter((t) => isTaskVisible(t, true, projectsById, tasksById));
	});
	const resolved = $derived<TaskNode[]>(nodes ?? (visibleTasks ? buildTree(visibleTasks) : []));
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each resolved as node (node.task.id)}
		<TaskItem
			task={node.task}
			{depth}
			{showProject}
			{hideTodayBadge}
			{hideTomorrowBadge}
			{hideDue}
			{draggable}
			{showUnplannedBadge}
			{mutator}
			{belongs}
			{onToggle}
		/>
		{#if node.children.length > 0}
			<Self
				nodes={node.children}
				depth={depth + 1}
				{showProject}
				{hideTodayBadge}
				{hideTomorrowBadge}
				{hideDue}
				{draggable}
				{showUnplannedBadge}
				{mutator}
				{belongs}
				{onToggle}
			/>
		{/if}
	{/each}
</div>
