<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Badge } from '$lib/components/ui/badge';
	import FlagIcon from 'phosphor-svelte/lib/Flag';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import SunDimIcon from 'phosphor-svelte/lib/SunDim';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import { PRIORITY_COLOR } from '$lib/utils/priority';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import LabelChips from './LabelChips.svelte';
	import DateBadge from './DateBadge.svelte';

	let {
		task,
		depth = 0,
		showProject = true,
		onToggle,
		onEdit,
		onDelete,
		onPinToggle
	}: {
		task: Task;
		depth?: number;
		showProject?: boolean;
		onToggle?: (task: Task) => void;
		onEdit?: (task: Task) => void;
		onDelete?: (task: Task) => void;
		onPinToggle?: (task: Task) => void;
	} = $props();

	const checked = $derived(task.status === 'completed');
	const project = $derived(
		task.projectId ? projectsStore.items.find((p) => p.id === task.projectId) : null
	);
	const overdue = $derived(
		task.status === 'open' && task.dueAt !== null && new Date(task.dueAt).getTime() < Date.now()
	);
</script>

<div
	class="group flex items-start gap-2 rounded px-2 py-1.5 hover:bg-muted/40"
	style:padding-left={`${depth * 1.25 + 0.5}rem`}
	data-task-id={task.id}
>
	<div class="pt-0.5">
		<Checkbox
			{checked}
			onCheckedChange={() => onToggle?.(task)}
			aria-label={checked ? 'Mark incomplete' : 'Mark complete'}
		/>
	</div>

	<div class="flex min-w-0 flex-1 flex-col gap-0.5">
		<div class="flex items-center gap-2">
			<FlagIcon class={`size-3 shrink-0 ${PRIORITY_COLOR[task.priority]}`} />
			<span
				class="min-w-0 flex-1 truncate text-sm"
				class:line-through={checked}
				class:text-muted-foreground={checked}
			>
				{task.title}
			</span>
			{#if task.isPinned}
				<PushPinIcon class="size-3 text-amber-500" />
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
			<DateBadge value={task.dueAt} hasTime={task.dueHasTime} {overdue} />
			{#if task.dayPart === 'morning'}
				<SunIcon class="size-3" />
			{:else if task.dayPart === 'afternoon'}
				<SunDimIcon class="size-3" />
			{:else if task.dayPart === 'evening'}
				<MoonIcon class="size-3" />
			{/if}
			{#if showProject && project}
				<Badge variant="outline" class="h-4 text-[10px]">{project.title}</Badge>
			{/if}
			{#if task.labels.length > 0}
				<LabelChips labels={task.labels} />
			{/if}
		</div>
	</div>

	<div class="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
		{#if onPinToggle}
			<button
				type="button"
				class="rounded p-1 hover:bg-muted"
				onclick={() => onPinToggle?.(task)}
				aria-label={task.isPinned ? 'Unpin' : 'Pin'}
			>
				<PushPinIcon class="size-3.5" />
			</button>
		{/if}
		{#if onEdit}
			<button
				type="button"
				class="rounded p-1 hover:bg-muted"
				onclick={() => onEdit?.(task)}
				aria-label="Edit"
			>
				<PencilIcon class="size-3.5" />
			</button>
		{/if}
		{#if onDelete}
			<button
				type="button"
				class="rounded p-1 hover:bg-muted hover:text-destructive"
				onclick={() => onDelete?.(task)}
				aria-label="Delete"
			>
				<TrashIcon class="size-3.5" />
			</button>
		{/if}
	</div>
</div>
