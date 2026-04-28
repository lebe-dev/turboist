<script lang="ts">
	import type { ProjectSection, Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { ListMutator } from '$lib/utils/taskActions';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import TrashIcon from 'phosphor-svelte/lib/Trash';

	let {
		section,
		tasks,
		mutator,
		belongs,
		onToggle,
		onRename,
		onRemove
	}: {
		section: ProjectSection;
		tasks: Task[];
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		onRename?: (section: ProjectSection) => void;
		onRemove?: (section: ProjectSection) => void;
	} = $props();

	let open = $state(true);
</script>

<section class="border-b border-border/50 last:border-b-0">
	<header class="flex items-center justify-between gap-2 px-3 py-2">
		<button
			type="button"
			class="flex flex-1 items-center gap-2 text-left text-sm font-medium hover:text-foreground"
			onclick={() => (open = !open)}
			aria-expanded={open}
		>
			{#if open}
				<CaretDownIcon class="size-3 text-muted-foreground" />
			{:else}
				<CaretRightIcon class="size-3 text-muted-foreground" />
			{/if}
			<span class="truncate">{section.title}</span>
			<span class="text-xs text-muted-foreground">{tasks.length}</span>
		</button>
		<div class="flex items-center gap-1">
			{#if onRename}
				<Button
					variant="ghost"
					size="icon"
					class="size-7"
					onclick={() => onRename?.(section)}
					aria-label="Rename section"
				>
					<PencilIcon class="size-3.5" />
				</Button>
			{/if}
			{#if onRemove}
				<Button
					variant="ghost"
					size="icon"
					class="size-7"
					onclick={() => onRemove?.(section)}
					aria-label="Delete section"
				>
					<TrashIcon class="size-3.5" />
				</Button>
			{/if}
		</div>
	</header>
	{#if open}
		{#if tasks.length === 0}
			<div class="px-6 py-2 text-xs text-muted-foreground">No tasks</div>
		{:else}
			<TaskTree
				{tasks}
				showProject={false}
				{mutator}
				{belongs}
				{onToggle}
			/>
		{/if}
	{/if}
</section>
