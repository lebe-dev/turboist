<script lang="ts">
	import { getContext } from 'svelte';
	import { resolve } from '$app/paths';
	import type { Task } from '$lib/api/types';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import RepeatIcon from 'phosphor-svelte/lib/Repeat';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { isOverdue } from '$lib/utils/format';
	import type { ListMutator } from '$lib/utils/taskActions';
	import LabelChips from './LabelChips.svelte';
	import DateBadge from './DateBadge.svelte';
	import PostponeBadge from './PostponeBadge.svelte';
	import TaskActionsMenu from './TaskActionsMenu.svelte';

	let {
		task,
		depth = 0,
		showProject = true,
		hideTodayBadge = false,
		hideTomorrowBadge = false,
		mutator,
		belongs,
		onToggle
	}: {
		task: Task;
		depth?: number;
		showProject?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
	} = $props();

	const getDayPartActive = getContext<(() => boolean) | undefined>('dayPartActive');
	const phaseActive = $derived(getDayPartActive ? getDayPartActive() : true);

	const checked = $derived(task.status === 'completed');
	const project = $derived(
		task.projectId ? projectsStore.items.find((p) => p.id === task.projectId) : null
	);
	const overdue = $derived(
		task.status === 'open' && isOverdue(task.dueAt, configStore.value?.timezone ?? null)
	);
	const taskHref = $derived(resolve('/(app)/task/[id]', { id: String(task.id) }));
	const description = $derived(task.description?.trim() ?? '');
	const isRecurring = $derived(!!task.recurrenceRule);
	const hasMeta = $derived(
		description.length > 0 ||
			!!task.dueAt ||
			(showProject && !!project) ||
			task.labels.length > 0 ||
			task.postponeCount >= 2 ||
			isRecurring
	);
</script>

<div
	class="group/task relative flex gap-3 rounded-lg px-3 transition-colors hover:bg-accent/50"
	class:items-start={hasMeta}
	class:items-center={!hasMeta}
	class:py-2.5={hasMeta}
	class:py-1.5={!hasMeta}
	style:padding-left={`${depth * 1.5 + 0.75}rem`}
	data-task-id={task.id}
>
	<button
		type="button"
		onclick={() => onToggle?.(task)}
		class="inline-flex size-4 shrink-0 items-center justify-center rounded-full border-[1.5px] transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
		class:mt-0.5={hasMeta}
		class:border-red-500={!checked && task.priority === 'high' && phaseActive}
		class:border-amber-500={!checked && task.priority === 'medium' && phaseActive}
		class:border-blue-500={!checked && task.priority === 'low' && phaseActive}
		class:border-border={!checked && (task.priority === 'no-priority' || !phaseActive)}
		class:hover:border-primary={!checked && task.priority === 'no-priority' && phaseActive}
		class:bg-primary={checked}
		class:border-primary={checked}
		class:text-primary-foreground={checked}
		aria-pressed={checked}
		aria-label={checked ? 'Mark incomplete' : 'Mark complete'}
	>
		{#if checked}
			<CheckIcon class="size-2.5" weight="bold" />
		{/if}
	</button>

	<div class="flex min-w-0 flex-1 flex-col gap-1">
		<div class="flex items-center gap-2">
			<a
				href={taskHref}
				class="min-w-0 flex-1 truncate text-sm leading-snug"
				class:font-medium={!checked}
				class:line-through={checked}
				class:text-muted-foreground={checked}
			>
				{task.title}
			</a>
		</div>

		{#if description}
			<p class="truncate text-xs text-muted-foreground/70">{description}</p>
		{/if}

		{#if isRecurring || task.dueAt || (showProject && project) || task.labels.length > 0 || task.postponeCount >= 2}
			<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
				{#if isRecurring}
					<span
						class="inline-flex items-center text-emerald-600 dark:text-emerald-400"
						title="Recurring task"
						aria-label="Recurring task"
					>
						<RepeatIcon class="size-3.5 shrink-0" weight="bold" />
					</span>
				{/if}
				<DateBadge
					value={task.dueAt}
					hasTime={task.dueHasTime}
					{overdue}
					{hideTodayBadge}
					{hideTomorrowBadge}
				/>
				<PostponeBadge count={task.postponeCount} />
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

	{#if mutator}
		<div class="flex items-center self-center">
			<TaskActionsMenu {task} {mutator} {belongs} />
		</div>
	{/if}
</div>
