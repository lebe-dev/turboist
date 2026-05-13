<script lang="ts">
	import type { ProjectSection, Task } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import SectionItem from './SectionItem.svelte';

	let {
		sections,
		tasksBySection,
		mutator,
		belongs,
		onToggle,
		collapseCompletedChildren = false,
		collapsibleSubtasks = false,
		onRenameSection,
		onRemoveSection,
		onAddSection,
		onSectionDrop,
		onTaskDrop
	}: {
		sections: ProjectSection[];
		tasksBySection: Record<number, Task[]>;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		collapseCompletedChildren?: boolean;
		collapsibleSubtasks?: boolean;
		onRenameSection?: (section: ProjectSection) => void;
		onRemoveSection?: (section: ProjectSection) => void;
		onAddSection?: (section: ProjectSection) => void;
		onSectionDrop?: (draggedId: number, targetId: number, before: boolean) => void;
		onTaskDrop?: (taskId: number, targetSectionId: number) => void;
	} = $props();
</script>

<div class="flex flex-col gap-2">
	{#each sections as section (section.id)}
		<SectionItem
			{section}
			tasks={tasksBySection[section.id] ?? []}
			{mutator}
			{belongs}
			{onToggle}
			{collapseCompletedChildren}
			{collapsibleSubtasks}
			onRename={onRenameSection}
			onRemove={onRemoveSection}
			onAddTask={onAddSection}
			{onSectionDrop}
			{onTaskDrop}
		/>
	{/each}
</div>
