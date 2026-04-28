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
		onRenameSection,
		onRemoveSection
	}: {
		sections: ProjectSection[];
		tasksBySection: Record<number, Task[]>;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		onRenameSection?: (section: ProjectSection) => void;
		onRemoveSection?: (section: ProjectSection) => void;
	} = $props();
</script>

<div class="flex flex-col">
	{#each sections as section (section.id)}
		<SectionItem
			{section}
			tasks={tasksBySection[section.id] ?? []}
			{mutator}
			{belongs}
			{onToggle}
			onRename={onRenameSection}
			onRemove={onRemoveSection}
		/>
	{/each}
</div>
