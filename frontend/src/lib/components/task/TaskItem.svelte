<script lang="ts">
	import type { Task } from '$lib/api/types';
	import FlagIcon from 'phosphor-svelte/lib/Flag';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import PencilSimpleIcon from 'phosphor-svelte/lib/PencilSimple';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import { PRIORITY_COLOR } from '$lib/utils/priority';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { isOverdue } from '$lib/utils/format';
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
		task.status === 'open' && isOverdue(task.dueAt, configStore.value?.timezone ?? null)
	);
	const showFlag = $derived(task.priority !== 'no-priority');
</script>

<div
	class="group/task relative flex items-start gap-3 rounded-lg px-3 py-2.5 transition-colors hover:bg-accent/50"
	style:padding-left={`${depth * 1.5 + 0.75}rem`}
	data-task-id={task.id}
>
	<button
		type="button"
		onclick={() => onToggle?.(task)}
		class="mt-[3px] inline-flex size-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
		class:border-border={!checked}
		class:hover:border-primary={!checked}
		class:bg-primary={checked}
		class:border-primary={checked}
		class:text-primary-foreground={checked}
		aria-pressed={checked}
		aria-label={checked ? 'Mark incomplete' : 'Mark complete'}
	>
		{#if checked}
			<CheckIcon class="size-3" weight="bold" />
		{/if}
	</button>

	<div class="flex min-w-0 flex-1 flex-col gap-1">
		<div class="flex items-center gap-2">
			{#if showFlag}
				<FlagIcon class={`size-4 shrink-0 ${PRIORITY_COLOR[task.priority]}`} weight="fill" />
			{/if}
			<span
				class="min-w-0 flex-1 truncate text-[15px] leading-snug"
				class:font-medium={!checked}
				class:line-through={checked}
				class:text-muted-foreground={checked}
			>
				{task.title}
			</span>
			{#if task.isPinned}
				<PushPinIcon class="size-3.5 shrink-0 text-amber-500" weight="fill" />
			{/if}
		</div>

		{#if task.dueAt || task.dayPart !== 'none' || (showProject && project) || task.labels.length > 0}
			<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
				<DateBadge value={task.dueAt} hasTime={task.dueHasTime} {overdue} />
				{#if task.dayPart === 'morning'}
					<span class="inline-flex items-center gap-1 text-muted-foreground" title="Morning">
						<SunHorizonIcon class="size-3.5" />
					</span>
				{:else if task.dayPart === 'afternoon'}
					<span class="inline-flex items-center gap-1 text-muted-foreground" title="Afternoon">
						<SunIcon class="size-3.5" />
					</span>
				{:else if task.dayPart === 'evening'}
					<span class="inline-flex items-center gap-1 text-muted-foreground" title="Evening">
						<MoonIcon class="size-3.5" />
					</span>
				{/if}
				{#if showProject && project}
					<span class="inline-flex items-center gap-1 text-muted-foreground">
						<FolderIcon class="size-3.5" />
						<span class="truncate">{project.title}</span>
					</span>
				{/if}
				{#if task.labels.length > 0}
					<LabelChips labels={task.labels} />
				{/if}
			</div>
		{/if}
	</div>

	<div
		class="flex items-center gap-0.5 opacity-0 transition-opacity group-hover/task:opacity-100 focus-within:opacity-100"
	>
		{#if onPinToggle}
			<button
				type="button"
				class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={() => onPinToggle?.(task)}
				aria-label={task.isPinned ? 'Unpin' : 'Pin'}
				title={task.isPinned ? 'Unpin' : 'Pin'}
			>
				<PushPinIcon class="size-4" weight={task.isPinned ? 'fill' : 'regular'} />
			</button>
		{/if}
		{#if onEdit}
			<button
				type="button"
				class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={() => onEdit?.(task)}
				aria-label="Edit"
				title="Edit"
			>
				<PencilSimpleIcon class="size-4" />
			</button>
		{/if}
		{#if onDelete}
			<button
				type="button"
				class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
				onclick={() => onDelete?.(task)}
				aria-label="Delete"
				title="Delete"
			>
				<TrashIcon class="size-4" />
			</button>
		{/if}
	</div>
</div>
