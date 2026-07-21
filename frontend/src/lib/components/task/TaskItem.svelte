<script lang="ts">
	import { getContext } from 'svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { Task } from '$lib/api/types';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import { Spinner } from '$lib/components/ui/spinner';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import RepeatIcon from 'phosphor-svelte/lib/Repeat';
	import CalendarSlashIcon from 'phosphor-svelte/lib/CalendarSlash';
	import CalendarCheckIcon from 'phosphor-svelte/lib/CalendarCheck';
	import ArchiveIcon from 'phosphor-svelte/lib/Archive';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import GaugeIcon from 'phosphor-svelte/lib/Gauge';
	import HourglassMediumIcon from 'phosphor-svelte/lib/HourglassMedium';
	import { t } from '$lib/i18n';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import { isOverdue } from '$lib/utils/format';
	import type { ListMutator } from '$lib/utils/taskActions';
	import LabelChips from './LabelChips.svelte';
	import DateBadge from './DateBadge.svelte';
	import PostponeBadge from './PostponeBadge.svelte';
	import TaskActionsMenu from './TaskActionsMenu.svelte';
	import MarkdownText from '$lib/components/MarkdownText.svelte';
	import { stripMarkdownSyntax } from '$lib/utils/markdown';
	import { SUBTASK_COLLAPSE_KEY, type SubtaskCollapseCtx } from '$lib/context/subtaskCollapse';
	import {
		setTaskDrag,
		clearTaskDrag,
		currentDraggingTaskId,
		hasDragKind,
		readDraggedTask,
		initTouchDrag,
		updateTouchDrag,
		endTouchDrag
	} from '$lib/utils/dnd';

	let {
		task,
		depth = 0,
		showProject = true,
		hideTodayBadge = false,
		hideTomorrowBadge = false,
		hideDue = false,
		draggable = false,
		showUnplannedBadge = false,
		mutator,
		belongs,
		onToggle,
		hasSubtasks = false,
		subtasksCollapsed = false,
		onToggleCollapse,
		onReparent,
		visibleIds
	}: {
		task: Task;
		depth?: number;
		showProject?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
		hideDue?: boolean;
		draggable?: boolean;
		showUnplannedBadge?: boolean;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void | Promise<void>;
		hasSubtasks?: boolean;
		subtasksCollapsed?: boolean;
		onToggleCollapse?: () => void;
		onReparent?: (draggedId: number, targetId: number) => void;
		visibleIds?: number[];
	} = $props();

	// When `onReparent` is provided, the row doubles as a drop target: dropping
	// another dragged task onto it re-parents that task as a sub-task of this one.
	const reparentEnabled = $derived(!!onReparent && draggable && !taskSelectionStore.mode);
	let dropAsChildActive = $state(false);

	const selected = $derived(taskSelectionStore.has(task.id));

	function onSelectionClick(e: MouseEvent): void {
		e.preventDefault();
		e.stopPropagation();
		if (e.shiftKey && visibleIds && visibleIds.length > 0) {
			taskSelectionStore.selectRange(visibleIds, task.id);
			return;
		}
		taskSelectionStore.toggle(task.id);
	}

	function onTaskDragStart(e: DragEvent) {
		setTaskDrag(e, task.id);
	}

	function onTaskDragEnd() {
		clearTaskDrag();
		dropAsChildActive = false;
	}

	function onChildDragOver(e: DragEvent) {
		if (!reparentEnabled || !hasDragKind(e, 'task')) return;
		// Don't offer the row as a drop target for itself.
		if (currentDraggingTaskId() === task.id) return;
		e.preventDefault();
		// Stop the section/root drop zone underneath from also highlighting.
		e.stopPropagation();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		dropAsChildActive = true;
	}

	function onChildDragLeave(e: DragEvent) {
		const target = e.currentTarget as HTMLElement;
		const related = e.relatedTarget as Node | null;
		if (related && target.contains(related)) return;
		dropAsChildActive = false;
	}

	function onChildDrop(e: DragEvent) {
		if (!reparentEnabled) return;
		const draggedId = readDraggedTask(e);
		dropAsChildActive = false;
		if (draggedId === null || draggedId === task.id) return;
		e.preventDefault();
		// Prevent the move from bubbling to the section drop handler, which would
		// instead place the task in the section and clear the new parent link.
		e.stopPropagation();
		onReparent?.(draggedId, task.id);
	}

	function onTaskTouchStart(e: TouchEvent) {
		if (!draggable || taskSelectionStore.mode) return;
		initTouchDrag(e, task.id, e.currentTarget as HTMLElement);
	}

	function onTaskTouchMove(e: TouchEvent) {
		if (!draggable || taskSelectionStore.mode) return;
		updateTouchDrag(e);
	}

	function onTaskTouchEnd(e: TouchEvent) {
		if (!draggable || taskSelectionStore.mode) return;
		const result = endTouchDrag(e);
		if (!result) return;
		window.dispatchEvent(new CustomEvent('turboist:task-touch-drop', { detail: result }));
	}

	const getDayPartActive = getContext<(() => boolean) | undefined>('dayPartActive');
	const phaseActive = $derived(getDayPartActive ? getDayPartActive() : true);

	// In the project view, "collapse all" doubles as a compact mode: hide task
	// descriptions. Only the project page sets this context, so other views
	// (today/week/inbox/search) leave descriptions untouched.
	const collapseCtx = getContext<SubtaskCollapseCtx | undefined>(SUBTASK_COLLAPSE_KEY);
	const hideDescription = $derived(!!collapseCtx?.allCollapsed);

	let descriptionExpanded = $state(false);

	// Show a spinner on the checkbox only when the toggle round-trip is slow
	// (e.g. poor connectivity). On a fast network it settles before the delay
	// elapses, so the spinner never flickers into view.
	let toggling = $state(false);

	async function handleToggle() {
		if (toggling) return;
		const result = onToggle?.(task);
		if (!(result instanceof Promise)) return;
		let settled = false;
		const timer = setTimeout(() => {
			if (!settled) toggling = true;
		}, 150);
		try {
			await result;
		} finally {
			settled = true;
			clearTimeout(timer);
			toggling = false;
		}
	}

	const checked = $derived(task.status === 'completed');
	// A negative id marks a task created offline and still queued in the outbox
	// (§4.5): show an unobtrusive "awaiting send" badge until replay assigns it a
	// real server id and the list refetches (§4.7.2). Visual only.
	const awaitingSend = $derived(task.id < 0);
	const project = $derived(
		task.projectId ? projectsStore.items.find((p) => p.id === task.projectId) : null
	);
	const overdue = $derived(
		task.status === 'open' && isOverdue(task.dueAt, configStore.value?.timezone ?? null)
	);
	const taskHref = $derived(resolve('/(app)/task/[id]', { id: String(task.id) }));
	const description = $derived(task.description?.trim() ?? '');
	const descriptionPreview = $derived(stripMarkdownSyntax(description));
	const descriptionExpandable = $derived(description.length > 100);
	const isRecurring = $derived(!!task.recurrenceRule || task.sourceTaskId !== null);
	const showTroikiBadge = $derived(
		settingsStore.troikiEnabled &&
			!!project?.troikiCategory &&
			page.url.pathname !== '/troiki' &&
			!page.url.pathname.startsWith('/task/') &&
			!page.url.pathname.startsWith('/project/')
	);
	const showWeekBadge = $derived(
		task.planState === 'week' &&
			page.url.pathname !== '/today' &&
			page.url.pathname !== '/week'
	);
	const showBacklogBadge = $derived(
		task.planState === 'backlog' && page.url.pathname !== '/next-week'
	);
	const showCalendarSlash = $derived(
		showUnplannedBadge &&
			task.planState !== 'week' &&
			(!settingsStore.troikiEnabled || !project?.troikiCategory) &&
			!task.labels.some((l) => settingsStore.weeklyUnplannedExcludedLabelIds.includes(l.id))
	);
	const hasMeta = $derived(
		description.length > 0 ||
			(!hideDue && !!task.dueAt) ||
			(showProject && !!project) ||
			task.labels.length > 0 ||
			task.postponeCount >= 2 ||
			isRecurring ||
			showCalendarSlash ||
			showWeekBadge ||
			showBacklogBadge
	);

	const checkboxClass = $derived.by(() => {
		const base =
			'inline-flex size-4 shrink-0 items-center justify-center rounded-full border-[1.5px] transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50';
		if (!checked) {
			if (task.priority === 'high' && phaseActive) return `${base} border-red-500`;
			if (task.priority === 'medium' && phaseActive) return `${base} border-amber-500`;
			if (task.priority === 'low' && phaseActive) return `${base} border-blue-500`;
			if (task.priority === 'no-priority' && phaseActive) return `${base} border-border hover:border-primary`;
			return `${base} border-border`;
		}
		const hoverBorder =
			task.priority === 'high'
				? 'group-hover/task:border-red-500 group-hover/task:bg-red-500'
				: task.priority === 'medium'
					? 'group-hover/task:border-amber-500 group-hover/task:bg-amber-500'
					: task.priority === 'low'
						? 'group-hover/task:border-blue-500 group-hover/task:bg-blue-500'
						: '';
		return `${base} bg-zinc-500 border-zinc-500 dark:bg-zinc-600 dark:border-zinc-600 text-white ${hoverBorder}`.trimEnd();
	});
</script>

<div
	class="group/task relative flex gap-3 rounded-lg px-3 transition-colors hover:bg-accent/50"
	class:items-start={hasMeta}
	class:items-center={!hasMeta}
	class:py-2.5={hasMeta}
	class:py-1.5={!hasMeta}
	class:bg-accent={taskSelectionStore.mode && selected}
	class:ring-2={dropAsChildActive}
	class:ring-inset={dropAsChildActive}
	class:ring-primary={dropAsChildActive}
	style:padding-left={onToggleCollapse ? `${depth * 1.5 + 0.25}rem` : `${depth * 1.5 + 0.75}rem`}
	data-task-id={task.id}
	draggable={draggable && !taskSelectionStore.mode}
	ondragstart={draggable && !taskSelectionStore.mode ? onTaskDragStart : undefined}
	ondragend={draggable && !taskSelectionStore.mode ? onTaskDragEnd : undefined}
	ondragover={reparentEnabled ? onChildDragOver : undefined}
	ondragleave={reparentEnabled ? onChildDragLeave : undefined}
	ondrop={reparentEnabled ? onChildDrop : undefined}
	ontouchstart={draggable && !taskSelectionStore.mode ? onTaskTouchStart : undefined}
	ontouchmove={draggable && !taskSelectionStore.mode ? onTaskTouchMove : undefined}
	ontouchend={draggable && !taskSelectionStore.mode ? onTaskTouchEnd : undefined}
	role={draggable ? 'listitem' : undefined}
>
	{#if onToggleCollapse}
		{#if hasSubtasks}
			<button
				type="button"
				onclick={onToggleCollapse}
				class="inline-flex size-4 shrink-0 items-center justify-center transition-colors {subtasksCollapsed
					? 'text-primary'
					: 'text-muted-foreground/50 hover:text-muted-foreground'}"
				class:mt-0.5={hasMeta}
				aria-label={subtasksCollapsed ? 'Развернуть субзадачи' : 'Свернуть субзадачи'}
				aria-expanded={!subtasksCollapsed}
			>
				{#if subtasksCollapsed}
					<CaretRightIcon class="size-3" />
				{:else}
					<CaretDownIcon class="size-3" />
				{/if}
			</button>
		{:else}
			<div class="size-4 shrink-0"></div>
		{/if}
	{/if}
	{#if taskSelectionStore.mode}
		<button
			type="button"
			onclick={onSelectionClick}
			class="inline-flex size-4 shrink-0 items-center justify-center rounded-[4px] border-[1.5px] transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			class:mt-0.5={hasMeta}
			class:border-primary={selected}
			class:bg-primary={selected}
			class:text-primary-foreground={selected}
			class:border-border={!selected}
			aria-pressed={selected}
			aria-label={selected ? $t('selection.unselectTask') : $t('selection.selectTask')}
		>
			{#if selected}
				<CheckIcon class="size-2.5" weight="bold" />
			{/if}
		</button>
	{/if}
	<button
		type="button"
		onclick={() => void handleToggle()}
		disabled={toggling}
		class={checkboxClass}
		class:mt-0.5={hasMeta}
		aria-pressed={checked}
		aria-busy={toggling}
		aria-label={checked ? $t('task.markIncomplete') : $t('task.markComplete')}
	>
		{#if toggling}
			<Spinner class="size-2.5" />
		{:else if checked}
			<CheckIcon class="size-2.5" weight="bold" />
		{/if}
	</button>

	<div class="flex min-w-0 flex-1 flex-col gap-1">
		<div class="flex items-center gap-2">
			<a
				href={taskHref}
				class="min-w-0 flex-1 break-words text-sm leading-snug md:truncate"
				class:font-medium={!checked}
				class:line-through={checked}
				class:text-muted-foreground={checked || depth > 0}
				class:text-foreground={!checked && depth === 0}
			>
				<MarkdownText text={task.title} linkClass="text-muted-foreground underline underline-offset-2 hover:text-foreground" />{#if task.isComplex}<span class="inline-flex align-middle" title={$t('task.complexTooltip')} aria-label={$t('task.complexMarker')}><GaugeIcon class="ml-1.5 inline-block size-3.5 text-red-500" weight="fill" /></span>{/if}{#if showTroikiBadge}<span title={$t('task.inTroikiTitle')} class="inline-block"><TroikiTriggerIcon class="ml-1.5 inline-block size-3 align-middle text-muted-foreground/50 transition-colors group-hover/task:text-primary" /></span>{/if}{#if task.isPrivate && !settingsStore.publicView}<span class="inline-flex align-middle" title={$t('common.privateTooltip')} aria-label={$t('common.privateMarker')}><LockSimpleIcon class="ml-1.5 inline-block size-2.5 text-muted-foreground/40" /></span>{/if}
			</a>
			{#if awaitingSend}
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-300"
					title={$t('offline.awaitingSend')}
				>
					<HourglassMediumIcon class="size-2.5" weight="fill" />
					{$t('offline.awaitingSend')}
				</span>
			{/if}
		</div>

		{#if descriptionPreview && !hideDescription}
			<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_noninteractive_element_interactions -->
			<p
				class="[overflow-wrap:anywhere] text-xs text-muted-foreground/70"
				class:line-clamp-2={!descriptionExpanded}
				class:cursor-pointer={descriptionExpandable && !descriptionExpanded}
				onclick={descriptionExpandable && !descriptionExpanded
					? (e) => { e.preventDefault(); e.stopPropagation(); descriptionExpanded = true; }
					: undefined}
			>{descriptionPreview}</p>
		{/if}

		{#if isRecurring || (!hideDue && task.dueAt) || (showProject && project) || task.labels.length > 0 || task.postponeCount >= 2 || showCalendarSlash || showWeekBadge || showBacklogBadge}
			<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
				{#if isRecurring}
					<span
						class="inline-flex items-center {checked
							? 'text-muted-foreground group-hover/task:text-emerald-600 dark:group-hover/task:text-emerald-400'
							: 'text-emerald-600 dark:text-emerald-400'}"
						title={$t('task.recurringLabel')}
						aria-label={$t('task.recurringLabel')}
					>
						<RepeatIcon class="size-3.5 shrink-0" weight="bold" />
					</span>
				{/if}
				{#if !hideDue}
					<DateBadge
						value={task.dueAt}
						hasTime={task.dueHasTime}
						{overdue}
						{hideTodayBadge}
						{hideTomorrowBadge}
						completed={checked}
					/>
				{/if}
				<PostponeBadge count={task.postponeCount} completed={checked} />
				{#if showProject && project}
					<span class="inline-flex items-center gap-1 text-muted-foreground">
						<FolderIcon class="size-3.5" />
						<span class="truncate">{project.title}</span>
					</span>
				{/if}
				{#if task.labels.length > 0}
					<LabelChips labels={task.labels} />
				{/if}
				{#if showCalendarSlash}
					<span
						class="inline-flex items-center {checked
							? 'text-muted-foreground group-hover/task:text-red-500'
							: 'text-red-500'}"
						title={$t('task.unplannedLabel')}
						aria-label={$t('task.unplannedLabel')}
					>
						<CalendarSlashIcon class="size-3.5 shrink-0" />
					</span>
				{/if}
				{#if showWeekBadge}
					<span
						class="inline-flex items-center {checked
							? 'text-muted-foreground/40'
							: 'text-muted-foreground/60'}"
						title={$t('task.weekPlannedLabel')}
						aria-label={$t('task.weekPlannedLabel')}
					>
						<CalendarCheckIcon class="size-3.5 shrink-0" />
					</span>
				{/if}
				{#if showBacklogBadge}
					<span
						class="inline-flex items-center {checked
							? 'text-muted-foreground/40'
							: 'text-violet-500'}"
						title={$t('task.backlogLabel')}
						aria-label={$t('task.backlogLabel')}
					>
						<ArchiveIcon class="size-3.5 shrink-0" />
					</span>
				{/if}
			</div>
		{/if}
	</div>

	{#if mutator}
		<div class="flex items-center self-center">
			<TaskActionsMenu {task} {mutator} {belongs} {hasSubtasks} />
		</div>
	{/if}
</div>
