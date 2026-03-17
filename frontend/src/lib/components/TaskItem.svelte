<script lang="ts">
	import TaskItem from './TaskItem.svelte';
	import type { Task } from '$lib/api/types';
	import { completeTask, deleteTask, duplicateTask, updateTask, getTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { labelFilterStore } from '$lib/stores/label-filter.svelte';
	import { nextActionStore } from '$lib/stores/next-action.svelte';
	import { toast } from 'svelte-sonner';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import MarkdownContent from './MarkdownContent.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import TaskDropdownMenu from './TaskDropdownMenu.svelte';
	import { portal } from '$lib/utils/portal';
	import { goto } from '$app/navigation';
	import { tick } from 'svelte';
	import { t, locale } from 'svelte-intl-precompile';

	import type { Snippet } from 'svelte';

	let { task, depth = 0, searchQuery = '', dimmed = false, hideTodayDue = false, hideTomorrowDue = false, completed = false, dropdownExtra, actionButton }: { task: Task; depth?: number; searchQuery?: string; dimmed?: boolean; hideTodayDue?: boolean; hideTomorrowDue?: boolean; completed?: boolean; dropdownExtra?: Snippet; actionButton?: Snippet } = $props();

	const priorityColor = $derived.by(() => {
		switch (task.priority) {
			case 4: return 'border-red-500';
			case 3: return 'border-amber-500';
			case 2: return 'border-blue-400';
			default: return 'border-muted-foreground/25';
		}
	});

	const priorityHoverColor = $derived.by(() => {
		switch (task.priority) {
			case 4: return 'hover:border-red-500 hover:bg-red-500/10';
			case 3: return 'hover:border-amber-500 hover:bg-amber-500/10';
			case 2: return 'hover:border-blue-400 hover:bg-blue-400/10';
			default: return 'hover:border-primary hover:bg-primary/10';
		}
	});

	const priorityCheckColor = $derived.by(() => {
		switch (task.priority) {
			case 4: return 'border-red-500 bg-red-500';
			case 3: return 'border-amber-500 bg-amber-500';
			case 2: return 'border-blue-400 bg-blue-400';
			default: return 'border-primary bg-primary';
		}
	});

	const visible = $derived(
		!searchQuery || task.content.toLowerCase().includes(searchQuery.toLowerCase())
	);

	const hasChildren = $derived(task.children && task.children.length > 0);
	const collapsed = $derived(collapsedStore.isCollapsed(task.id));

	// Find task by ID recursively in the tree
	function findTaskInStore(tasks: import('$lib/api/types').Task[], id: string): import('$lib/api/types').Task | null {
		for (const t of tasks) {
			if (t.id === id) return t;
			const found = findTaskInStore(t.children, id);
			if (found) return found;
		}
		return null;
	}

	const isCompletedView = $derived(contextsStore.activeView === 'completed');

	let completing = $state(false);

	async function handleComplete() {
		if (completing) return;
		completing = true;

		// Capture parent info before removing from store
		const parentId = task.parent_id;
		let parentContent: string | null = null;
		if (parentId && !isCompletedView) {
			const parent = findTaskInStore(tasksStore.tasks, parentId);
			parentContent = parent?.content ?? null;
			if (!parentContent) {
				try {
					const fetched = await getTask(parentId);
					parentContent = fetched.content;
				} catch {
					// fallback — skip next-action prompt
				}
			}
		}
		const completedTask = task;

		await new Promise((r) => setTimeout(r, 200));
		tasksStore.removeTaskLocal(task.id);
		completing = false;

		// Show next-action toast
		if (!isCompletedView) {
			const isSubtask = parentId && parentContent;
			const isLeafTask = !parentId && completedTask.sub_task_count === 0 && completedTask.completed_sub_task_count === 0;

			if (isSubtask || isLeafTask) {
				toast.dismiss();
				toast(`Completed: ${completedTask.content}`, {
					duration: 8000,
					action: {
						label: isSubtask ? 'Next action' : 'Follow-up',
						onClick: () => {
							if (isSubtask) {
								nextActionStore.trigger(completedTask, parentContent!);
							} else {
								nextActionStore.triggerFollowUp(completedTask);
							}
						}
					}
				});
			}
		}

		try {
			await completeTask(task.id);
		} catch (e) {
			console.error('Failed to complete task', e);
			tasksStore.clearPendingRemoval(task.id);
			tasksStore.refresh();
		}
	}

	const dueLabel = $derived.by(() => {
		if (!task.due) return null;
		const d = new Date(task.due.date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const tomorrow = new Date(today);
		tomorrow.setDate(tomorrow.getDate() + 1);
		if (d.getTime() === today.getTime()) return hideTodayDue ? null : $t('due.today');
		if (d.getTime() === tomorrow.getTime()) return hideTomorrowDue ? null : $t('due.tomorrow');
		const loc = $locale === 'ru' ? 'ru-RU' : 'en-US';
		return d.toLocaleDateString(loc, { day: 'numeric', month: 'short' });
	});

	const isOverdue = $derived.by(() => {
		if (!task.due) return false;
		const d = new Date(task.due.date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		return d < today;
	});

	const completedAtLabel = $derived.by(() => {
		if (!task.completed_at) return null;
		const d = new Date(task.completed_at);
		const loc = $locale === 'ru' ? 'ru-RU' : 'en-US';
		return d.toLocaleDateString(loc, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
	});

	const dayPartLabels = $derived(new Set(tasksStore.config?.day_parts?.map((dp) => dp.label) ?? []));
	const visibleLabels = $derived(task.labels.filter((l) => !dayPartLabels.has(l)));

	const isPinned = $derived(pinnedStore.isPinned(task.id));
	const canPin = $derived(isPinned || !pinnedStore.isFull);

	const backlogLabel = $derived(tasksStore.config?.backlog_label ?? '');
	const isInBacklog = $derived(backlogLabel !== '' && task.labels.includes(backlogLabel));

	async function toggleBacklog() {
		if (!backlogLabel) return;
		dropdownOpen = false;

		const newLabels = isInBacklog
			? task.labels.filter((l) => l !== backlogLabel)
			: [...task.labels, backlogLabel];

		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, labels: newLabels }));
		try {
			await updateTask(task.id, { labels: newLabels });
		} catch (e) {
			console.error('Failed to toggle backlog label', e);
		}
		tasksStore.refresh();
	}

	function handlePin() {
		if (isPinned) {
			pinnedStore.unpin(task.id);
		} else {
			pinnedStore.pin({ id: task.id, content: task.content });
		}
	}

	// --- Date helpers ---
	async function setDate(date: string) {
		dropdownOpen = false;
		if (task.due?.date === date) return;
		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, due: { date, recurring: false } }));
		try {
			await updateTask(task.id, { due_date: date });
		} catch (e) {
			console.error('Failed to set due date', e);
		}
		tasksStore.refresh();
	}

	async function clearDate() {
		dropdownOpen = false;
		if (!task.due) return;
		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, due: null }));
		try {
			await updateTask(task.id, { due_date: '' });
		} catch (e) {
			console.error('Failed to clear due date', e);
		}
		tasksStore.refresh();
	}

	let dateInput: HTMLInputElement | undefined = $state();

	function openDatePicker() {
		requestAnimationFrame(() => {
			dateInput?.showPicker?.();
			dateInput?.focus();
		});
	}

	async function onDatePicked(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		if (value) await setDate(value);
	}

	// --- Priority ---
	async function setPriority(value: number) {
		dropdownOpen = false;
		if (task.priority === value) return;
		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, priority: value }));
		try {
			await updateTask(task.id, { priority: value });
		} catch (e) {
			console.error('Failed to update priority', e);
		}
		tasksStore.refresh();
	}

	let dropdownOpen = $state(false);

	// --- Long-press to open dropdown ---
	let longPressTimer: ReturnType<typeof setTimeout> | null = null;
	let longPressTriggered = false;

	function handleTouchStart() {
		longPressTriggered = false;
		longPressTimer = setTimeout(() => {
			longPressTimer = null;
			longPressTriggered = true;
			dropdownOpen = true;
		}, 500);
	}

	function handleTouchEnd(e: TouchEvent) {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
		if (longPressTriggered) {
			e.preventDefault();
			longPressTriggered = false;
		}
	}

	function handleTouchMove() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	// --- Duplicate ---
	let duplicating = $state(false);

	async function handleDuplicate() {
		if (duplicating) return;
		duplicating = true;

		// Snapshot task data and ID before dropdown closes
		const sourceId = task.id;
		const taskContent = task.content;
		const tempId = `temp-dup-${Date.now()}`;
		const clone: import('$lib/api/types').Task = {
			...$state.snapshot(task),
			id: tempId,
			children: [],
			sub_task_count: 0,
			completed_sub_task_count: 0,
		};

		// Let bits-ui close the dropdown first, then insert
		dropdownOpen = false;
		await tick();
		tasksStore.insertAfterLocal(sourceId, clone);

		try {
			const newId = await duplicateTask(sourceId);
			toast.dismiss();
			toast(`Duplicated: ${taskContent}`, {
				duration: 5000,
				action: {
					label: 'Open',
					onClick: () => goto(`/task/${newId}`)
				}
			});
		} catch (e) {
			console.error('Failed to duplicate task', e);
			tasksStore.removeTaskLocal(tempId);
			toast.error('Failed to duplicate task');
		} finally {
			duplicating = false;
		}
	}

	// --- Label filter ---
	function handleLabelClick(label: string) {
		contextsStore.setContext(null);
		contextsStore.setView('all');
		labelFilterStore.set(label);
		goto('/');
	}

	// --- Delete ---
	let showDeleteConfirm = $state(false);
	let deleting = $state(false);

	async function handleDelete() {
		if (deleting) return;
		deleting = true;
		tasksStore.removeTaskLocal(task.id);
		showDeleteConfirm = false;
		try {
			await deleteTask(task.id);
		} catch (e) {
			console.error('Failed to delete task', e);
			tasksStore.clearPendingRemoval(task.id);
			tasksStore.refresh();
		} finally {
			deleting = false;
		}
	}
</script>

{#if task.is_project_task}
	<div class="mt-6 first:mt-0">
		<div class="mb-1.5 flex items-center gap-2 px-2 md:gap-3 md:px-3">
			<div class="h-px flex-1 bg-border/60"></div>
			<h3 class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				{task.content}
			</h3>
			<div class="h-px flex-1 bg-border/60"></div>
		</div>
		{#if task.children.length > 0}
			<div>
				{#each task.children as child (child.id)}
					<TaskItem task={child} depth={0} {searchQuery} />
				{/each}
			</div>
		{/if}
	</div>
{:else if visible}
	<div style="padding-left: {depth * 16}px">
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="group relative flex items-center gap-2 rounded-lg px-2 py-1.5 transition-colors duration-150 hover:bg-accent/50 md:gap-3 md:px-3 md:py-2 select-none"
			class:opacity-40={completing}
			class:scale-[0.99]={completing}
			ontouchstart={!completed ? handleTouchStart : undefined}
			ontouchend={!completed ? handleTouchEnd : undefined}
			ontouchmove={!completed ? handleTouchMove : undefined}
			ontouchcancel={!completed ? handleTouchEnd : undefined}
			oncontextmenu={!completed ? (e) => e.preventDefault() : undefined}
		>
			{#if completed}
				<span class="flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] border-muted-foreground/30 bg-muted-foreground/10">
					<CheckIcon class="h-2.5 w-2.5 text-muted-foreground/60" strokeWidth={3} />
				</span>
			{:else}
				<button
					class="flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
						{completing
						? priorityCheckColor
						: dimmed
							? 'border-muted-foreground/40 hover:border-muted-foreground/60 hover:bg-muted-foreground/5'
							: priorityColor + ' ' + priorityHoverColor}"
					style="-webkit-tap-highlight-color: transparent;"
					onclick={handleComplete}
					disabled={completing}
					aria-label="Complete task"
				>
					{#if completing}
						<CheckIcon class="h-2.5 w-2.5 text-primary-foreground" strokeWidth={3} />
					{:else}
						<CheckIcon class="h-2.5 w-2.5 text-primary opacity-0 transition-opacity duration-150 group-hover:opacity-50" strokeWidth={3} />
					{/if}
				</button>
			{/if}

			{#snippet taskContentInner()}
				<MarkdownContent text={task.content} class="break-words text-[13px] leading-relaxed {completed ? 'line-through text-muted-foreground' : 'text-foreground/90'}" />
				{#if task.description && !completed}
					<p class="truncate text-[12px] text-muted-foreground"><MarkdownContent text={task.description} /></p>
				{/if}
				{#if completed && completedAtLabel}
					<p class="text-[11px] text-muted-foreground/60">{completedAtLabel}</p>
				{:else if visibleLabels.length > 0 || task.due || task.sub_task_count > 0}
					<div class="mt-0.5 flex flex-wrap items-center gap-1 md:mt-1 md:gap-1.5">
						{#if dueLabel}
							<span
								class="flex items-center gap-1 text-[11px] {isOverdue
									? 'text-destructive'
									: 'text-muted-foreground'}"
							>
								<CalendarIcon class="h-3 w-3" />
								{dueLabel}
							</span>
						{/if}
						{#each visibleLabels as label (label)}
							{#if completed}
								<span class="rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">{label}</span>
							{:else}
								<button
									class="rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
									onclick={(e) => { e.preventDefault(); e.stopPropagation(); handleLabelClick(label); }}
								>{label}</button>
							{/if}
						{/each}
						{#if task.sub_task_count > 0}
							<button
								class="flex items-center gap-0.5 text-[11px] tabular-nums text-muted-foreground hover:text-foreground transition-colors"
								onclick={(e) => { e.preventDefault(); collapsedStore.toggle(task.id); }}
								aria-label={collapsed ? 'Expand subtasks' : 'Collapse subtasks'}
							>
								<ChevronRightIcon
									class="h-3 w-3 transition-transform duration-150 {collapsed ? '' : 'rotate-90'}"
								/>
								{task.completed_sub_task_count}/{task.sub_task_count}
							</button>
						{/if}
					</div>
				{/if}
			{/snippet}

			{#if completed}
				<div class="min-w-0 flex-1 overflow-hidden">
					{@render taskContentInner()}
				</div>
			{:else}
				<a href="/task/{task.id}" class="min-w-0 flex-1 cursor-pointer overflow-hidden" style="-webkit-touch-callout: none;">
					{@render taskContentInner()}
				</a>
			{/if}

			{#if actionButton}
				{@render actionButton()}
			{/if}

			{#if !completed}
				<input
					bind:this={dateInput}
					type="date"
					value={task.due?.date ?? ''}
					class="pointer-events-none absolute left-0 top-0 h-0 w-0 opacity-0"
					onchange={onDatePicked}
				/>
				<TaskDropdownMenu
					bind:open={dropdownOpen}
					{task}
					onEdit={() => goto(`/task/${task.id}`)}
					onDuplicate={handleDuplicate}
					onCopy={() => { navigator.clipboard.writeText(task.content); dropdownOpen = false; }}
					{canPin}
					{isPinned}
					onPin={handlePin}
					{backlogLabel}
					{isInBacklog}
					onToggleBacklog={toggleBacklog}
					{dropdownExtra}
					onSetDate={setDate}
					onClearDate={clearDate}
					onOpenDatePicker={openDatePicker}
					onSetPriority={setPriority}
					onDelete={() => { dropdownOpen = false; showDeleteConfirm = true; }}
				>
					{#snippet trigger()}
						<DropdownMenu.Trigger
							class="absolute right-1 top-1/2 -translate-y-1/2 flex h-8 w-8 md:h-6 md:w-6 items-center justify-center rounded text-muted-foreground/40 transition-all duration-150 md:opacity-0 md:group-hover:opacity-100 hover:text-muted-foreground"
							onclick={(e: MouseEvent) => e.stopPropagation()}
							ontouchstart={(e: TouchEvent) => e.stopPropagation()}
							ontouchend={(e: TouchEvent) => e.stopPropagation()}
						>
							<EllipsisIcon class="h-5 w-5" />
						</DropdownMenu.Trigger>
					{/snippet}
				</TaskDropdownMenu>
			{/if}
		</div>

			{#if hasChildren && !collapsed && !completed}
			<div>
				{#each task.children as child (child.id)}
					<TaskItem task={child} depth={depth + 1} {searchQuery} {dimmed} {hideTodayDue} {hideTomorrowDue} />
				{/each}
			</div>
		{/if}
	</div>
{/if}

{#if showDeleteConfirm}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		use:portal
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		onclick={() => { showDeleteConfirm = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showDeleteConfirm = false; }}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-lg border border-border bg-background p-6 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold text-foreground">{$t('task.deleteConfirm')}</h3>
			<p class="mt-2 truncate text-sm text-muted-foreground">
				{$t('task.deleteDescription', { values: { name: task.content } })}
			</p>
			<div class="mt-4 flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					onclick={() => { showDeleteConfirm = false; }}
				>
					{$t('dialog.cancel')}
				</button>
				<button
					class="rounded-md bg-destructive px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-destructive/90"
					onclick={handleDelete}
					disabled={deleting}
				>
					{$t('dialog.delete')}
				</button>
			</div>
		</div>
	</div>
{/if}
