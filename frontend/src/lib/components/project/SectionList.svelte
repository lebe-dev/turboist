<script lang="ts">
	import type { ProjectSection, Task } from '$lib/api/types';
	import SectionItem from './SectionItem.svelte';

	let {
		sections,
		tasksBySection,
		onToggle,
		onEdit,
		onDelete,
		onPinToggle,
		onRenameSection,
		onRemoveSection
	}: {
		sections: ProjectSection[];
		tasksBySection: Record<number, Task[]>;
		onToggle?: (task: Task) => void;
		onEdit?: (task: Task) => void;
		onDelete?: (task: Task) => void;
		onPinToggle?: (task: Task) => void;
		onRenameSection?: (section: ProjectSection) => void;
		onRemoveSection?: (section: ProjectSection) => void;
	} = $props();
</script>

<div class="flex flex-col">
	{#each sections as section (section.id)}
		<SectionItem
			{section}
			tasks={tasksBySection[section.id] ?? []}
			{onToggle}
			{onEdit}
			{onDelete}
			{onPinToggle}
			onRename={onRenameSection}
			onRemove={onRemoveSection}
		/>
	{/each}
</div>
