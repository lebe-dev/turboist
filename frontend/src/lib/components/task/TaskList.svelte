<script lang="ts">
	import type { Task } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import {
		buildProjectsById,
		buildTasksById,
		isTaskVisible
	} from '$lib/utils/visibility';
	import TaskItem from './TaskItem.svelte';

	let {
		tasks,
		showProject = true,
		mutator,
		belongs,
		onToggle
	}: {
		tasks: Task[];
		showProject?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
	} = $props();

	const visibleTasks = $derived.by(() => {
		if (!settingsStore.publicView) return tasks;
		const tasksById = buildTasksById(tasks);
		const projectsById = buildProjectsById(projectsStore.items ?? []);
		return tasks.filter((t) => isTaskVisible(t, true, projectsById, tasksById));
	});
</script>

<div class="flex flex-col divide-y divide-border/40">
	{#each visibleTasks as task (task.id)}
		<TaskItem {task} {showProject} {mutator} {belongs} {onToggle} />
	{/each}
</div>
