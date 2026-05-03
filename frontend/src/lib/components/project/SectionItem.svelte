<script lang="ts">
	import type { ProjectSection, Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import CompletedTasksGroup from './CompletedTasksGroup.svelte';
	import { Button } from '$lib/components/ui/button';
	import { splitByRootCompletion } from '$lib/utils/taskTree';
	import type { ListMutator } from '$lib/utils/taskActions';
	import {
		hasDragKind,
		isUpperHalf,
		readDraggedSection,
		readDraggedTask,
		setSectionDrag
	} from '$lib/utils/dnd';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import DotsSixVerticalIcon from 'phosphor-svelte/lib/DotsSixVertical';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import TrashIcon from 'phosphor-svelte/lib/Trash';

	let {
		section,
		tasks,
		mutator,
		belongs,
		onToggle,
		onRename,
		onRemove,
		onSectionDrop,
		onTaskDrop,
		taskDraggable = true
	}: {
		section: ProjectSection;
		tasks: Task[];
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		onRename?: (section: ProjectSection) => void;
		onRemove?: (section: ProjectSection) => void;
		onSectionDrop?: (draggedId: number, targetId: number, before: boolean) => void;
		onTaskDrop?: (taskId: number, targetSectionId: number) => void;
		taskDraggable?: boolean;
	} = $props();

	let open = $state(true);
	const split = $derived(splitByRootCompletion(tasks));
	let dragIndicator = $state<'none' | 'top' | 'bottom' | 'task'>('none');
	let sectionEl = $state<HTMLElement | null>(null);
	let dragging = $state(false);

	function onHeaderDragStart(e: DragEvent) {
		setSectionDrag(e, section.id);
		dragging = true;
	}

	function onHeaderDragEnd() {
		dragging = false;
		dragIndicator = 'none';
	}

	function onDragOver(e: DragEvent) {
		if (dragging) return;
		const isSection = hasDragKind(e, 'section');
		const isTask = hasDragKind(e, 'task');
		if (!isSection && !isTask) return;
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		if (isSection && sectionEl) {
			const rect = sectionEl.getBoundingClientRect();
			dragIndicator = isUpperHalf(e, rect) ? 'top' : 'bottom';
		} else if (isTask) {
			dragIndicator = 'task';
		}
	}

	function onDragLeave(e: DragEvent) {
		if (!sectionEl) return;
		const related = e.relatedTarget as Node | null;
		if (related && sectionEl.contains(related)) return;
		dragIndicator = 'none';
	}

	function onDrop(e: DragEvent) {
		const sectionId = readDraggedSection(e);
		if (sectionId !== null) {
			e.preventDefault();
			if (sectionId !== section.id && sectionEl) {
				const before = isUpperHalf(e, sectionEl.getBoundingClientRect());
				onSectionDrop?.(sectionId, section.id, before);
			}
			dragIndicator = 'none';
			return;
		}
		const taskId = readDraggedTask(e);
		if (taskId !== null) {
			e.preventDefault();
			onTaskDrop?.(taskId, section.id);
			dragIndicator = 'none';
		}
	}
</script>

<section
	bind:this={sectionEl}
	class={[
		'relative border-b border-border/50 last:border-b-0',
		dragIndicator === 'task' && 'bg-accent/40'
	]}
	ondragover={onDragOver}
	ondragleave={onDragLeave}
	ondrop={onDrop}
	aria-label={section.title}
>
	{#if dragIndicator === 'top'}
		<div class="pointer-events-none absolute inset-x-0 top-0 h-0.5 bg-primary"></div>
	{:else if dragIndicator === 'bottom'}
		<div class="pointer-events-none absolute inset-x-0 bottom-0 h-0.5 bg-primary"></div>
	{/if}
	<header
		class="flex items-center justify-between gap-2 px-3 py-2"
		draggable="true"
		ondragstart={onHeaderDragStart}
		ondragend={onHeaderDragEnd}
		role="group"
		aria-roledescription="Draggable section"
	>
		<button
			type="button"
			class="flex flex-1 items-center gap-2 text-left text-sm font-medium hover:text-foreground"
			onclick={() => (open = !open)}
			aria-expanded={open}
		>
			<DotsSixVerticalIcon class="size-3.5 text-muted-foreground/60 cursor-grab" />
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
			{#if split.open.length > 0}
				<TaskTree
					tasks={split.open}
					showProject={false}
					draggable={taskDraggable}
					{mutator}
					{belongs}
					{onToggle}
				/>
			{/if}
			{#if split.done.length > 0}
				<CompletedTasksGroup
					tasks={split.done}
					draggable={taskDraggable}
					{mutator}
					{belongs}
					{onToggle}
				/>
			{/if}
		{/if}
	{/if}
</section>
