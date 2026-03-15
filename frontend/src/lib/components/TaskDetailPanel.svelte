<script lang="ts">
	import type { Task, Label } from '$lib/api/types';
	import { updateTask, createTask, completeTask, deleteTask, duplicateTask, getLabels, getTask, getCompletedSubtasks } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import { nextActionStore } from '$lib/stores/next-action.svelte';
	import { toast } from 'svelte-sonner';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import XIcon from '@lucide/svelte/icons/x';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import TagIcon from '@lucide/svelte/icons/tag';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import RepeatIcon from '@lucide/svelte/icons/repeat';
	import TrashIcon from '@lucide/svelte/icons/trash-2';
	import CopyPlusIcon from '@lucide/svelte/icons/copy-plus';
	import PinIcon from '@lucide/svelte/icons/pin';
	import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import SunIcon from '@lucide/svelte/icons/sun';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import MarkdownContent from './MarkdownContent.svelte';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Calendar } from '$lib/components/ui/calendar';
	import { parseDate, type DateValue } from '@internationalized/date';
	import { t, locale } from 'svelte-intl-precompile';

	let {
		taskId,
		onclose,
		onselect,
		fullPage = false
	}: {
		taskId: string;
		onclose: () => void;
		onselect?: (id: string) => void;
		fullPage?: boolean;
	} = $props();

	// Find task by ID recursively in the tree
	function findTask(tasks: Task[], id: string): Task | null {
		for (const t of tasks) {
			if (t.id === id) return t;
			const found = findTask(t.children, id);
			if (found) return found;
		}
		return null;
	}

	const taskFromStore = $derived(findTask(tasksStore.tasks, taskId));
	let taskFromApi = $state<Task | null>(null);
	let taskFetching = $state(false);

	// Always fetch full task from API (store version may have filtered children)
	$effect(() => {
		taskFetching = true;
		taskFromApi = null;
		getTask(taskId)
			.then((t) => { taskFromApi = t; })
			.catch(() => { if (!taskFromStore) onclose(); })
			.finally(() => { taskFetching = false; });
	});

	// Prefer API version (has all children); fall back to store for instant display
	// Guard: ignore stale taskFromApi when taskId has changed but $effect hasn't re-run yet
	const task = $derived((taskFromApi?.id === taskId ? taskFromApi : null) ?? taskFromStore);

	function updateLocal(updater: (t: Task) => Task) {
		if (taskFromApi) taskFromApi = updater(taskFromApi);
		if (task) tasksStore.updateTaskLocal(task.id, updater);
	}

	let parentTask = $state<Task | null>(null);

	// Stable primitives derived from task — prevents effects from re-running on every store update
	const taskParentId = $derived(task?.parent_id ?? null);
	const taskCompletedSubCount = $derived(task?.completed_sub_task_count ?? 0);
	const taskLabelsKey = $derived(task ? task.labels.join(',') : null);

	$effect(() => {
		const pid = taskParentId;
		if (!pid) {
			parentTask = null;
			return;
		}
		const found = findTask(tasksStore.tasks, pid);
		if (found) {
			parentTask = found;
			return;
		}
		getTask(pid).then((t) => (parentTask = t)).catch(() => (parentTask = null));
	});

	// Close panel if task disappears (e.g. completed) — skip during API fetch
	$effect(() => {
		if (!task && !taskFetching) onclose();
	});

	// --- Completed subtasks ---
	let completedSubtasks = $state<Task[]>([]);
	let completedCollapsed = $state(true);

	$effect(() => {
		const id = taskId;
		const count = taskCompletedSubCount;
		if (!id || count === 0) {
			completedSubtasks = [];
			return;
		}
		getCompletedSubtasks(id)
			.then((tasks) => { completedSubtasks = tasks; })
			.catch(() => { completedSubtasks = []; });
	});

	// --- Title editing ---
	let editingTitle = $state(false);
	let titleValue = $state('');
	let titleInput: HTMLTextAreaElement | null = $state(null);

	function startEditTitle() {
		if (!task) return;
		titleValue = task.content;
		editingTitle = true;
		requestAnimationFrame(() => {
			if (!titleInput) return;
			titleInput.focus();
			const len = titleValue.length;
			titleInput.setSelectionRange(len, len);
		});
	}

	async function saveTitle() {
		if (!task || !editingTitle) return;
		editingTitle = false;
		const trimmed = titleValue.trim();
		if (!trimmed || trimmed === task.content) return;
		updateLocal((t) => ({ ...t, content: trimmed }));
		try {
			await updateTask(task.id, { content: trimmed });
		} catch (e) {
			console.error('Failed to update title', e);
		}
		tasksStore.refresh();
	}

	function cancelTitle() {
		editingTitle = false;
	}

	// --- Description editing ---
	let editingDesc = $state(false);
	let descValue = $state('');
	let descInput: HTMLTextAreaElement | undefined = $state();

	function startEditDesc() {
		if (!task) return;
		descValue = task.description;
		editingDesc = true;
		requestAnimationFrame(() => {
			if (descInput) {
				descInput.focus();
				descInput.style.height = 'auto';
				descInput.style.height = descInput.scrollHeight + 'px';
			}
		});
	}

	async function saveDesc() {
		if (!task || !editingDesc) return;
		editingDesc = false;
		const trimmed = descValue.trim();
		if (trimmed === task.description) return;
		updateLocal((t) => ({ ...t, description: trimmed }));
		try {
			await updateTask(task.id, { description: trimmed });
		} catch (e) {
			console.error('Failed to update description', e);
		}
		tasksStore.refresh();
	}

	function cancelDesc() {
		editingDesc = false;
	}

	// --- Priority (optimistic) ---
	let showPriorityPicker = $state(false);
	let localPriority = $state(1);
	let prioritySyncing = $state(false);

	$effect(() => {
		if (task && !prioritySyncing) {
			localPriority = task.priority;
		}
	});

	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500', border: 'border-red-500' },
		{ value: 3, label: 'P2', color: 'text-amber-500', border: 'border-amber-500' },
		{ value: 2, label: 'P3', color: 'text-blue-400', border: 'border-blue-400' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground', border: 'border-muted-foreground/25' }
	];

	const activePriority = $derived(priorityItems.find((p) => p.value === localPriority));

	async function setPriority(value: number) {
		if (!task) return;
		dropdownOpen = false;
		showPriorityPicker = false;
		if (value === localPriority) return;
		localPriority = value;
		prioritySyncing = true;
		try {
			await updateTask(task.id, { priority: value });
			tasksStore.refresh();
		} catch (e) {
			if (task) localPriority = task.priority;
			console.error('Failed to update priority', e);
		} finally {
			prioritySyncing = false;
		}
	}

	// --- Due date ---
	let showCalendar = $state(false);

	let calendarValue = $state<DateValue | undefined>(undefined);

	$effect(() => {
		if (task?.due?.date) {
			calendarValue = parseDate(task.due.date);
		} else {
			calendarValue = undefined;
		}
	});

	async function onCalendarSelect(value: DateValue | undefined) {
		if (!task || !value) return;
		const dateStr = `${value.year}-${String(value.month).padStart(2, '0')}-${String(value.day).padStart(2, '0')}`;
		const currentDate = task.due?.date ?? '';
		if (dateStr === currentDate) {
			showCalendar = false;
			return;
		}
		showCalendar = false;
		updateLocal((t) => ({ ...t, due: { date: dateStr, recurring: t.due?.recurring ?? false } }));
		try {
			await updateTask(task.id, { due_date: dateStr });
		} catch (e) {
			console.error('Failed to update due date', e);
		}
		tasksStore.refresh();
	}

	async function clearDate() {
		if (!task || !task.due) return;
		dropdownOpen = false;
		updateLocal((t) => ({ ...t, due: null }));
		try {
			await updateTask(task.id, { due_date: '' });
		} catch (e) {
			console.error('Failed to clear due date', e);
		}
		tasksStore.refresh();
	}

	function todayDateStr(): string {
		const d = new Date();
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	function tomorrowDateStr(): string {
		const d = new Date();
		d.setDate(d.getDate() + 1);
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	async function setDateQuick(date: string) {
		if (!task) return;
		dropdownOpen = false;
		const currentDate = task.due?.date ?? '';
		if (date === currentDate) return;
		updateLocal((t) => ({ ...t, due: { date, recurring: false } }));
		try {
			await updateTask(task.id, { due_date: date });
		} catch (e) {
			console.error('Failed to set due date', e);
		}
		tasksStore.refresh();
	}

	// --- Labels (optimistic) ---
	let allLabels = $state<Label[]>([]);
	let showLabelPicker = $state(false);
	let labelSearch = $state('');
	let localLabels = $state<string[]>([]);
	let labelsSyncing = $state(false);

	// Sync local labels from store when task labels change (skip during pending API call)
	$effect(() => {
		const key = taskLabelsKey;
		if (key !== null && !labelsSyncing) {
			localLabels = key ? key.split(',') : [];
		}
	});

	onMount(() => {
		getLabels().then((labels) => { allLabels = labels; }).catch(() => {});

		const mq = window.matchMedia('(max-width: 767px)');
		isMobile = mq.matches;
		const handleMq = (e: MediaQueryListEvent) => { isMobile = e.matches; };
		mq.addEventListener('change', handleMq);
		return () => mq.removeEventListener('change', handleMq);
	});

	const filteredLabels = $derived.by(() => {
		if (!labelSearch) return allLabels;
		const q = labelSearch.toLowerCase();
		return allLabels.filter((l) => l.name.toLowerCase().includes(q));
	});

	const contextLabels = $derived.by(() => {
		const ctxId = contextsStore.activeContextId;
		if (!ctxId) return [];
		const ctx = contextsStore.contexts.find((c) => c.id === ctxId);
		return ctx?.filters.labels ?? [];
	});

	async function toggleLabel(name: string) {
		if (!task) return;
		const newLabels = localLabels.includes(name)
			? localLabels.filter((l) => l !== name)
			: [...localLabels, name];
		localLabels = newLabels;
		labelsSyncing = true;
		try {
			await updateTask(task.id, { labels: newLabels });
			tasksStore.refresh();
		} catch (e) {
			if (task) localLabels = [...task.labels];
			console.error('Failed to update labels', e);
		} finally {
			labelsSyncing = false;
		}
	}

	// --- Complete task ---
	let completing = $state(false);

	const isCompletedView = $derived(contextsStore.activeView === 'completed');

	async function handleComplete(id?: string) {
		const targetId = id ?? task?.id;
		if (!targetId || completing) return;
		completing = true;
		// Optimistic: short delay for animation, then remove locally
		await new Promise((r) => setTimeout(r, 200));
		if (targetId === task?.id) {
			// Completing the main task — remove and navigate to parent or close
			const parentId = task?.parent_id ?? null;

			// Show next-action toast
			if (!isCompletedView) {
				const completedTask = { ...task! };
				const isSubtask = parentId && parentTask?.content;
				const isLeafTask = !parentId && completedTask.sub_task_count === 0 && completedTask.completed_sub_task_count === 0;

				if (isSubtask || isLeafTask) {
					toast.dismiss();
					toast(`Completed: ${completedTask.content}`, {
						duration: 8000,
						action: {
							label: isSubtask ? 'Next action' : 'Follow-up',
							onClick: () => {
								if (isSubtask) {
									nextActionStore.trigger(completedTask, parentTask!.content);
								} else {
									nextActionStore.triggerFollowUp(completedTask);
								}
							}
						}
					});
				}
			}

			tasksStore.removeTaskLocal(targetId);
			onclose();
		} else {
			// Completing a subtask — capture info before removing
			const child = task?.children.find((c) => c.id === targetId);
			if (child && task && !isCompletedView) {
				const completedChild = { ...child, parent_id: child.parent_id ?? task.id };
				const parentName = task.content;
				toast.dismiss();
				toast(`Completed: ${completedChild.content}`, {
					duration: 8000,
					action: {
						label: 'Next action',
						onClick: () => {
							nextActionStore.trigger(completedChild, parentName);
						}
					}
				});
			}

			updateLocal((t) => ({
				...t,
				children: t.children.filter((c) => c.id !== targetId),
				sub_task_count: Math.max(0, t.sub_task_count - 1),
				completed_sub_task_count: t.completed_sub_task_count + 1
			}));
		}
		completing = false;
		try {
			await completeTask(targetId);
		} catch (e) {
			console.error('Failed to complete task', e);
			tasksStore.clearPendingRemoval(targetId);
			tasksStore.refresh();
		}
	}

	// --- Subtask menu ---
	let openSubtaskMenuId = $state<string | null>(null);

	// --- Subtask priority (optimistic) ---
	async function setSubtaskPriority(childId: string, value: number) {
		if (!task) return;
		openSubtaskMenuId = null;
		updateLocal((t) => ({
			...t,
			children: t.children
				.map((c) => (c.id === childId ? { ...c, priority: value } : c))
				.sort((a, b) => b.priority - a.priority)
		}));
		try {
			await updateTask(childId, { priority: value });
		} catch (e) {
			console.error('Failed to update subtask priority', e);
		}
		tasksStore.refresh();
	}

	// --- Subtask date ---
	async function setSubtaskDate(childId: string, date: string) {
		openSubtaskMenuId = null;
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) =>
				c.id === childId ? { ...c, due: { date, recurring: false } } : c
			)
		}));
		try {
			await updateTask(childId, { due_date: date });
		} catch (e) {
			console.error('Failed to set subtask date', e);
		}
		tasksStore.refresh();
	}

	async function clearSubtaskDate(childId: string) {
		openSubtaskMenuId = null;
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) =>
				c.id === childId ? { ...c, due: null } : c
			)
		}));
		try {
			await updateTask(childId, { due_date: '' });
		} catch (e) {
			console.error('Failed to clear subtask date', e);
		}
		tasksStore.refresh();
	}

	let subtaskCalendarTargetId = $state<string | null>(null);

	function openSubtaskDatePicker(childId: string) {
		subtaskCalendarTargetId = subtaskCalendarTargetId === childId ? null : childId;
	}

	async function onSubtaskCalendarSelect(value: DateValue | undefined) {
		if (!value || !subtaskCalendarTargetId) return;
		const dateStr = `${value.year}-${String(value.month).padStart(2, '0')}-${String(value.day).padStart(2, '0')}`;
		const targetId = subtaskCalendarTargetId;
		subtaskCalendarTargetId = null;
		await setSubtaskDate(targetId, dateStr);
	}

	// --- Delete subtask ---
	async function deleteSubtask(childId: string) {
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.filter((c) => c.id !== childId),
			sub_task_count: Math.max(0, t.sub_task_count - 1)
		}));
		try {
			await deleteTask(childId);
		} catch (e) {
			console.error('Failed to delete subtask', e);
		}
		tasksStore.refresh();
	}

	// --- Add sub-task ---
	let showSubtaskForm = $state(false);
	let subtaskContent = $state('');
	let subtaskTextarea: HTMLTextAreaElement | undefined = $state();
	let addingSubtask = $state(false);
	let isMobile = $state(false);

	function extractPrefix(content: string): string {
		const match = content.match(/^(.+?(?::\s|\s-\s))/);
		return match ? match[1] : '';
	}

	function detectPrefixFromSiblings(children: Task[]): string {
		const prefixes: Record<string, number> = {};
		for (const child of children) {
			const p = extractPrefix(child.content);
			if (p) prefixes[p] = (prefixes[p] ?? 0) + 1;
		}
		let best = '';
		let bestCount = 0;
		for (const [p, count] of Object.entries(prefixes)) {
			if (count > bestCount) {
				best = p;
				bestCount = count;
			}
		}
		return best;
	}

	function startAddSubtask() {
		if (!task) return;
		const siblingPrefix = detectPrefixFromSiblings(task.children);
		const prefix = siblingPrefix || extractPrefix(task.content);
		subtaskContent = prefix;
		showSubtaskForm = true;
		requestAnimationFrame(() => {
			subtaskTextarea?.focus();
		});
	}

	async function saveSubtask() {
		if (!task || !subtaskContent.trim() || addingSubtask) return;
		addingSubtask = true;
		const content = subtaskContent.trim();
		const labels = [...new Set([...task.labels, ...contextLabels])];
		const tempId = `temp-${Date.now()}`;
		const optimistic: Task = {
			id: tempId,
			content,
			description: '',
			project_id: task.project_id,
			section_id: task.section_id,
			parent_id: task.id,
			labels,
			priority: 1,
			due: null,
			sub_task_count: 0,
			completed_sub_task_count: 0,
			completed_at: null,
			added_at: new Date().toISOString(),
			is_project_task: false,
			children: []
		};
		updateLocal((t) => ({
			...t,
			children: [...t.children, optimistic],
			sub_task_count: t.sub_task_count + 1
		}));
		subtaskContent = '';
		showSubtaskForm = false;
		addingSubtask = false;
		try {
			await createTask(
				{ content, description: '', labels, priority: 1, parent_id: task.id },
				contextsStore.activeContextId ?? undefined
			);
		} catch (e) {
			console.error('Failed to create subtask', e);
		}
		tasksStore.refresh();
	}

	// --- Due date display ---
	function formatDueDate(date: string): string {
		const d = new Date(date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const tomorrow = new Date(today);
		tomorrow.setDate(tomorrow.getDate() + 1);
		if (d.getTime() === today.getTime()) return $t('due.today');
		if (d.getTime() === tomorrow.getTime()) return $t('due.tomorrow');
		const loc = $locale === 'ru' ? 'ru-RU' : 'en-US';
		return d.toLocaleDateString(loc, { day: 'numeric', month: 'short', year: 'numeric' });
	}

	function isOverdue(date: string): boolean {
		const d = new Date(date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		return d < today;
	}

	// --- Priority circle color ---
	function priorityBorder(p: number): string {
		switch (p) {
			case 4: return 'border-red-500';
			case 3: return 'border-amber-500';
			case 2: return 'border-blue-400';
			default: return 'border-muted-foreground/25';
		}
	}

	function priorityHover(p: number): string {
		switch (p) {
			case 4: return 'hover:border-red-500 hover:bg-red-500/10';
			case 3: return 'hover:border-amber-500 hover:bg-amber-500/10';
			case 2: return 'hover:border-blue-400 hover:bg-blue-400/10';
			default: return 'hover:border-primary hover:bg-primary/10';
		}
	}

	// --- Keyboard ---
	function isEditingText(): boolean {
		const el = document.activeElement;
		if (!el) return false;
		const tag = el.tagName;
		return tag === 'INPUT' || tag === 'TEXTAREA' || (el as HTMLElement).isContentEditable;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (showCalendar) {
				showCalendar = false;
			} else if (subtaskCalendarTargetId) {
				subtaskCalendarTargetId = null;
			} else if (showLabelPicker) {
				showLabelPicker = false;
			} else if (showPriorityPicker) {
				showPriorityPicker = false;
			} else if (editingTitle) {
				editingTitle = false;
			} else if (editingDesc) {
				editingDesc = false;
			} else if (showSubtaskForm) {
				showSubtaskForm = false;
			} else {
				onclose();
			}
			e.stopPropagation();
			return;
		}

		if ((e.key === 'q' || e.key === 'Q') && !e.ctrlKey && !e.metaKey && !e.altKey && !isEditingText()) {
			e.preventDefault();
			startAddSubtask();
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onclose();
		}
	}

	const collapsed = $derived(task ? collapsedStore.isCollapsed(task.id) : false);

	// --- Dropdown state ---
	let dropdownOpen = $state(false);

	// --- Duplicate task ---
	let duplicating = $state(false);

	async function handleDuplicate() {
		if (!task || duplicating) return;
		duplicating = true;
		dropdownOpen = false;
		const taskContent = task.content;
		const tempId = `temp-dup-${Date.now()}`;
		const clone: Task = {
			...task,
			id: tempId,
			children: [],
			sub_task_count: 0,
			completed_sub_task_count: 0,
		};
		tasksStore.insertAfterLocal(task.id, clone);
		try {
			const newId = await duplicateTask(task.id);
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
		}
		tasksStore.refresh();
		duplicating = false;
	}

	// --- Pin ---
	const isPinned = $derived(task ? pinnedStore.isPinned(task.id) : false);
	const canPin = $derived(isPinned || !pinnedStore.isFull);

	function handlePin() {
		if (!task) return;
		dropdownOpen = false;
		if (isPinned) {
			pinnedStore.unpin(task.id);
		} else {
			pinnedStore.pin({ id: task.id, content: task.content });
		}
	}

	// --- Delete task ---
	let showDeleteConfirm = $state(false);
	let deleting = $state(false);

	async function handleDelete() {
		if (!task || deleting) return;
		deleting = true;
		tasksStore.removeTaskLocal(task.id);
		showDeleteConfirm = false;
		onclose();
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

<svelte:window onkeydown={handleKeydown} />

{#if task}
	{#snippet panelContent()}
		<!-- Header -->
		<div class="flex shrink-0 items-center justify-between border-b border-border/50 px-5 py-3">
			<div class="flex items-center gap-2 text-[12px] text-muted-foreground">
				{#if fullPage}
					{#if task.parent_id && parentTask}
						<button
							class="flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
							onclick={() => onselect?.(parentTask!.id)}
						>
							<ArrowLeftIcon class="h-3.5 w-3.5" />
							<span class="truncate">{parentTask.content}</span>
						</button>
					{:else}
						<button
							class="flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
							onclick={onclose}
						>
							<ArrowLeftIcon class="h-3.5 w-3.5" />
						</button>
					{/if}
				{:else if task.parent_id && parentTask}
					<button
						class="flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:bg-accent hover:text-foreground"
						onclick={() => onselect?.(parentTask!.id)}
					>
						<ChevronRightIcon class="h-3 w-3 rotate-180" />
						{parentTask.content}
					</button>
				{/if}
			</div>
			<div class="flex items-center gap-1">
				<DropdownMenu.Root bind:open={dropdownOpen}>
					<DropdownMenu.Trigger
						class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					>
						<EllipsisVerticalIcon class="h-4 w-4" />
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end" class="w-52">
						<!-- Duplicate -->
						<DropdownMenu.Item onclick={handleDuplicate} disabled={duplicating}>
							<CopyPlusIcon class="h-4 w-4" />
							{$t('task.duplicate')}
						</DropdownMenu.Item>

						{#if canPin}
							<DropdownMenu.Item onclick={handlePin}>
								<PinIcon class="h-4 w-4" />
								{isPinned ? $t('task.unpin') : $t('task.pin')}
							</DropdownMenu.Item>
						{/if}

						<DropdownMenu.Separator />

						<!-- Date -->
						<div class="px-2 py-1.5">
							<p class="text-xs font-semibold text-muted-foreground">{$t('task.date')}</p>
							<div class="mt-1.5 flex items-center gap-1">
								<button
									class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
										{task.due?.date === todayDateStr() ? 'bg-accent text-green-500' : 'text-green-500 hover:bg-accent'}"
									onclick={() => setDateQuick(todayDateStr())}
									aria-label="Today"
								>
									<CalendarIcon class="h-4 w-4" />
								</button>
								<button
									class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
										{task.due?.date === tomorrowDateStr() ? 'bg-accent text-amber-500' : 'text-amber-500 hover:bg-accent'}"
									onclick={() => setDateQuick(tomorrowDateStr())}
									aria-label="Tomorrow"
								>
									<SunIcon class="h-4 w-4" />
								</button>
								{#if task.due}
									<button
										class="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
										onclick={clearDate}
										aria-label="Clear date"
									>
										<XIcon class="h-3.5 w-3.5" />
									</button>
								{/if}
							</div>
						</div>

						<!-- Priority -->
						<div class="px-2 py-1.5">
							<p class="text-xs font-semibold text-muted-foreground">{$t('task.priority')}</p>
							<div class="mt-1.5 flex items-center gap-1">
								{#each priorityItems as p (p.value)}
									<button
										class="flex h-7 w-7 items-center justify-center rounded-md transition-colors {p.color}
											{localPriority === p.value ? 'bg-accent' : 'hover:bg-accent'}"
										onclick={() => setPriority(p.value)}
										aria-label={p.label}
									>
										<FlagIcon class="h-4 w-4" />
									</button>
								{/each}
							</div>
						</div>

						<DropdownMenu.Separator />

						<!-- Delete -->
						<DropdownMenu.Item
							variant="destructive"
							onclick={() => { dropdownOpen = false; showDeleteConfirm = true; }}
						>
							<TrashIcon class="h-4 w-4" />
							{$t('dialog.delete')}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
				{#if !fullPage}
					<button
						class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						onclick={onclose}
						aria-label="Close"
					>
						<XIcon class="h-4 w-4" />
					</button>
				{/if}
			</div>
		</div>

			<!-- Content -->
			<div class="flex min-h-0 flex-1 overflow-hidden">
				<!-- Left: main content -->
				<div class="flex-1 overflow-y-auto p-6">
					<!-- Title with complete button -->
					<div class="flex items-start gap-3">
						<button
							class="mt-1 flex h-[20px] w-[20px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
								{completing
									? 'border-primary bg-primary'
									: priorityBorder(localPriority) + ' ' + priorityHover(localPriority)}"
							onclick={() => handleComplete()}
							disabled={completing}
							aria-label="Complete task"
						>
							{#if completing}
								<CheckIcon class="h-3 w-3 text-primary-foreground" strokeWidth={3} />
							{/if}
						</button>

						{#if editingTitle}
							<div class="flex-1">
								<Textarea
									bind:ref={titleInput}
									bind:value={titleValue}
									class="min-h-0 w-full resize-none rounded-none border-none bg-transparent p-0 text-lg md:text-lg font-semibold leading-snug text-foreground shadow-none focus-visible:ring-0"
									onkeydown={(e) => {
										if (e.key === 'Enter' && !e.shiftKey) {
											e.preventDefault();
											saveTitle();
										}
									}}
								/>
								<div class="mt-2 flex items-center gap-2">
									<button
										class="rounded-md bg-primary px-3 py-1 text-[12px] font-medium text-primary-foreground transition-colors hover:bg-primary/90"
										onclick={saveTitle}
									>{$t('dialog.save')}</button>
									<button
										class="rounded-md px-3 py-1 text-[12px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
										onclick={cancelTitle}
									>{$t('dialog.cancel')}</button>
								</div>
							</div>
						{:else}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<h2
								class="flex-1 cursor-text text-lg font-semibold leading-snug text-foreground"
								onclick={startEditTitle}
							>
								<MarkdownContent text={task.content} />
							</h2>
						{/if}
					</div>

					<!-- Description -->
					<div class="mt-4 pl-8">
						{#if editingDesc}
							<textarea
								bind:this={descInput}
								bind:value={descValue}
								class="w-full resize-none rounded-md border border-border/50 bg-transparent p-2 text-sm text-foreground placeholder:text-muted-foreground/40 focus:border-border focus:outline-none"
								placeholder={$t('task.addDescription')}
								rows="3"
								oninput={(e) => {
									const target = e.currentTarget;
									target.style.height = 'auto';
									target.style.height = target.scrollHeight + 'px';
								}}
							></textarea>
							<div class="mt-2 flex items-center gap-2">
								<button
									class="rounded-md bg-primary px-3 py-1 text-[12px] font-medium text-primary-foreground transition-colors hover:bg-primary/90"
									onclick={saveDesc}
								>{$t('dialog.save')}</button>
								<button
									class="rounded-md px-3 py-1 text-[12px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
									onclick={cancelDesc}
								>{$t('dialog.cancel')}</button>
							</div>
						{:else}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								class="cursor-text rounded-md py-1.5 text-sm transition-colors hover:bg-accent/50
									{task.description ? 'text-foreground/80' : 'text-muted-foreground/40'}"
								onclick={startEditDesc}
							>
								{#if task.description}
									<p class="whitespace-pre-wrap"><MarkdownContent text={task.description} /></p>
								{:else}
									Add description...
								{/if}
							</div>
						{/if}
					</div>

					<!-- Subtasks -->
					{#if task.children.length > 0}
						<div class="mt-6">
							<div class="mb-2 flex items-center gap-2">
								<button
									class="flex items-center gap-1 text-[12px] tabular-nums text-muted-foreground transition-colors hover:text-foreground"
									onclick={() => collapsedStore.toggle(task.id)}
								>
									<ChevronRightIcon
										class="h-3.5 w-3.5 transition-transform duration-150 {collapsed ? '' : 'rotate-90'}"
									/>
									Subtasks {task.completed_sub_task_count}/{task.sub_task_count}
								</button>
							</div>
							{#if !collapsed}
								<div class="space-y-0.5">
									{#each task.children as child (child.id)}
										<div class="group relative flex items-center gap-2.5 rounded-lg px-2 py-1.5 transition-colors hover:bg-accent/50">
											<button
												class="flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
													{priorityBorder(child.priority)} {priorityHover(child.priority)}"
												onclick={() => handleComplete(child.id)}
												aria-label="Complete subtask"
											>
												<CheckIcon class="h-2 w-2 text-primary opacity-0 transition-opacity group-hover:opacity-50" strokeWidth={3} />
											</button>
											<!-- svelte-ignore a11y_click_events_have_key_events -->
											<!-- svelte-ignore a11y_no_static_element_interactions -->
											<div
												class="min-w-0 flex-1 {onselect ? 'cursor-pointer' : ''}"
												onclick={() => onselect?.(child.id)}
											>
												<MarkdownContent text={child.content} class="text-[13px] text-foreground/90" />
												<div class="mt-1 flex items-center gap-2">
													{#if child.due}
														<span class="flex items-center gap-0.5 text-[11px] {isOverdue(child.due.date) ? 'text-destructive' : 'text-muted-foreground'}">
															<CalendarIcon class="h-3 w-3" />
															{formatDueDate(child.due.date)}
														</span>
													{/if}
													{#each child.labels as label (label)}
														<span class="rounded px-1.5 py-0.5 text-[11px] bg-muted text-muted-foreground">{label}</span>
													{/each}
												</div>
											</div>
											<DropdownMenu.Root open={openSubtaskMenuId === child.id} onOpenChange={(v) => { openSubtaskMenuId = v ? child.id : null; }}>
												<DropdownMenu.Trigger
													class="absolute right-1 top-1/2 -translate-y-1/2 flex h-6 w-6 items-center justify-center rounded text-muted-foreground/40 opacity-0 transition-all duration-150 group-hover:opacity-100 hover:text-muted-foreground"
													onclick={(e: MouseEvent) => e.stopPropagation()}
												>
													<EllipsisIcon class="h-4 w-4" />
												</DropdownMenu.Trigger>
												<DropdownMenu.Content align="end" class="w-64">
													<DropdownMenu.Item onclick={() => onselect?.(child.id)}>
														<PencilIcon class="h-4 w-4" />
														{$t('task.edit')}
													</DropdownMenu.Item>

													<DropdownMenu.Separator />

													<!-- Date -->
													<div class="px-2 py-1.5">
														<p class="text-xs font-semibold text-muted-foreground">{$t('task.date')}</p>
														<div class="mt-1.5 flex items-center gap-1">
															<button
																class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
																	{child.due?.date === todayDateStr() ? 'bg-accent text-green-500' : 'text-green-500 hover:bg-accent'}"
																onclick={() => setSubtaskDate(child.id, todayDateStr())}
																aria-label="Today"
															>
																<CalendarIcon class="h-4 w-4" />
															</button>
															<button
																class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
																	{child.due?.date === tomorrowDateStr() ? 'bg-accent text-amber-500' : 'text-amber-500 hover:bg-accent'}"
																onclick={() => setSubtaskDate(child.id, tomorrowDateStr())}
																aria-label="Tomorrow"
															>
																<SunIcon class="h-4 w-4" />
															</button>
															<button
															class="flex h-7 w-7 items-center justify-center rounded-md text-purple-400 transition-colors hover:bg-accent"
															onclick={() => openSubtaskDatePicker(child.id)}
															aria-label="Pick date"
														>
															<ArrowRightIcon class="h-4 w-4" />
														</button>
															{#if child.due}
																<button
																	class="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
																	onclick={() => clearSubtaskDate(child.id)}
																	aria-label="Clear date"
																>
																	<XIcon class="h-3.5 w-3.5" />
																</button>
															{/if}
														</div>
														{#if subtaskCalendarTargetId === child.id}
															<div class="mt-1">
																<Calendar
																	type="single"
																	value={child.due?.date ? parseDate(child.due.date) : undefined}
																	onValueChange={onSubtaskCalendarSelect}
																	class="rounded-md border border-border"
																/>
															</div>
														{/if}
													</div>

													<!-- Priority -->
													<div class="px-2 py-1.5">
														<p class="text-xs font-semibold text-muted-foreground">{$t('task.priority')}</p>
														<div class="mt-1.5 flex items-center gap-1">
															{#each priorityItems as p (p.value)}
																<button
																	class="flex h-7 w-7 items-center justify-center rounded-md transition-colors {p.color}
																		{child.priority === p.value ? 'bg-accent' : 'hover:bg-accent'}"
																	onclick={() => setSubtaskPriority(child.id, p.value)}
																	aria-label={p.label}
																>
																	<FlagIcon class="h-4 w-4" />
																</button>
															{/each}
														</div>
													</div>

													<DropdownMenu.Separator />

													<DropdownMenu.Item
														variant="destructive"
														onclick={() => deleteSubtask(child.id)}
													>
														<TrashIcon class="h-4 w-4" />
														{$t('dialog.delete')}
													</DropdownMenu.Item>
												</DropdownMenu.Content>
											</DropdownMenu.Root>
										</div>
									{/each}
								</div>
								{/if}
						</div>
					{/if}

					<!-- Add sub-task -->
					<div class="mt-4">
						<button
							class="flex items-center gap-2 text-[13px] text-muted-foreground transition-colors hover:text-primary"
							onclick={startAddSubtask}
						>
							<PlusIcon class="h-4 w-4" />
							{$t('task.addSubtask')}
						</button>
					</div>

					<!-- Subtask dialog -->
					{#if showSubtaskForm}
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
							onclick={(e) => {
								if (e.target === e.currentTarget) {
									showSubtaskForm = false;
									subtaskContent = '';
								}
							}}
							onkeydown={(e) => {
								if (e.key === 'Escape') {
									showSubtaskForm = false;
									subtaskContent = '';
								}
							}}
						>
							<div class="w-full max-w-sm mx-4 rounded-xl border border-border bg-popover shadow-2xl">
								<div class="px-4 pt-4 pb-2">
									<textarea
										bind:this={subtaskTextarea}
										bind:value={subtaskContent}
										placeholder={$t('task.subtaskName')}
										rows="2"
										class="w-full resize-none bg-transparent text-base leading-snug text-foreground placeholder:text-muted-foreground/40 focus:outline-none"
										disabled={addingSubtask}
										onkeydown={(e) => {
											if (e.key === 'Enter' && !e.shiftKey) {
												e.preventDefault();
												saveSubtask();
											}
											if (e.key === 'Escape') {
												showSubtaskForm = false;
												subtaskContent = '';
											}
										}}
									></textarea>
								</div>
								<div class="flex items-center justify-end gap-2 border-t border-border/50 px-4 py-3">
									<button
										class="rounded-lg px-4 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
										onclick={() => { showSubtaskForm = false; subtaskContent = ''; }}
									>
										{$t('dialog.cancel')}
									</button>
									<button
										class="rounded-lg px-4 py-1.5 text-[13px] font-medium transition-colors
											{subtaskContent.trim()
												? 'bg-primary text-primary-foreground hover:bg-primary/90'
												: 'bg-muted text-muted-foreground cursor-not-allowed'}"
										disabled={!subtaskContent.trim() || addingSubtask}
										onclick={saveSubtask}
									>
										{addingSubtask ? '...' : $t('task.add')}
									</button>
								</div>
							</div>
						</div>
					{/if}

					<!-- Completed subtasks -->
					{#if completedSubtasks.length > 0}
						<div class="mt-4">
							<button
								class="flex items-center gap-1 text-[12px] tabular-nums text-muted-foreground transition-colors hover:text-foreground"
								onclick={() => (completedCollapsed = !completedCollapsed)}
							>
								<ChevronRightIcon
									class="h-3.5 w-3.5 transition-transform duration-150 {completedCollapsed ? '' : 'rotate-90'}"
								/>
								Completed {completedSubtasks.length}
							</button>
							{#if !completedCollapsed}
								<div class="mt-2 space-y-0.5">
									{#each completedSubtasks as child (child.id)}
										<div class="flex items-start gap-2.5 rounded-lg px-2 py-1.5">
											<div class="mt-0.5 flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-full border-[1.5px] border-primary bg-primary">
												<CheckIcon class="h-2 w-2 text-primary-foreground" strokeWidth={3} />
											</div>
											<div class="min-w-0 flex-1">
												<span class="text-[13px] text-muted-foreground line-through">{child.content}</span>
												{#if child.completed_at}
													<p class="mt-0.5 text-[11px] text-muted-foreground/60">
														{new Date(child.completed_at).toLocaleDateString($locale === 'ru' ? 'ru-RU' : 'en-US', { day: 'numeric', month: 'short' })}
													</p>
												{/if}
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Right: sidebar -->
				<div class="hidden w-72 shrink-0 space-y-5 overflow-y-auto border-l border-border/50 p-5 md:block">
					<!-- Date -->
					<div>
						<h3 class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">Date</h3>
						<div class="flex items-center gap-1.5">
							<button
								class="rounded-md px-2.5 py-1 text-[12px] transition-colors
									{task.due?.date === todayDateStr() ? 'bg-accent text-foreground font-medium' : 'text-muted-foreground hover:bg-accent'}"
								onclick={() => setDateQuick(todayDateStr())}
							>Today</button>
							<button
								class="rounded-md px-2.5 py-1 text-[12px] transition-colors
									{task.due?.date === tomorrowDateStr() ? 'bg-accent text-foreground font-medium' : 'text-muted-foreground hover:bg-accent'}"
								onclick={() => setDateQuick(tomorrowDateStr())}
							>Tomorrow</button>
							<button
								class="flex items-center justify-center rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
								onclick={() => (showCalendar = !showCalendar)}
								aria-label="Pick custom date"
							>
								<CalendarIcon class="h-3.5 w-3.5" />
							</button>
							{#if task.due}
								<button
									class="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/50 transition-colors hover:bg-accent hover:text-foreground"
									onclick={clearDate}
									aria-label="Clear date"
								>
									<XIcon class="h-3 w-3" />
								</button>
							{/if}
						</div>
						{#if showCalendar}
							<div class="mt-2">
								<Calendar
									type="single"
									value={calendarValue}
									onValueChange={onCalendarSelect}
									class="rounded-md border border-border"
								/>
							</div>
						{/if}
					</div>

					<!-- Priority -->
					<div>
						<h3 class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">{$t('task.priority')}</h3>
						<div class="relative">
							<button
								class="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] transition-colors hover:bg-accent {activePriority?.color}"
								onclick={() => (showPriorityPicker = !showPriorityPicker)}
							>
								<FlagIcon class="h-4 w-4" />
								{activePriority?.label ?? 'P4'}
							</button>

							{#if showPriorityPicker}
								<div class="absolute left-0 top-full z-10 mt-1 w-36 rounded-lg border border-border bg-popover shadow-xl">
									<div class="px-1 py-1">
										{#each priorityItems as p (p.value)}
											<button
												class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12px] transition-colors hover:bg-accent
													{localPriority === p.value ? 'bg-accent' : ''}"
												onclick={() => setPriority(p.value)}
											>
												<FlagIcon class="h-3.5 w-3.5 {p.color}" />
												<span class={p.color}>{p.label}</span>
											</button>
										{/each}
									</div>
								</div>
							{/if}
						</div>
					</div>

					<!-- Labels -->
					<div>
						<h3 class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">{$t('task.labels')}</h3>
						{#if localLabels.length > 0}
							<div class="mb-2 flex flex-wrap gap-1.5">
								{#each localLabels as label (label)}
									<button
										class="flex items-center gap-1 rounded-md px-2 py-0.5 text-[12px] font-medium transition-colors
											{contextLabels.includes(label)
												? 'bg-primary/10 text-primary'
												: 'bg-muted text-muted-foreground hover:bg-muted/80'}"
										onclick={() => toggleLabel(label)}
									>
										{label}
										{#if !contextLabels.includes(label)}
											<XIcon class="h-3 w-3" />
										{/if}
									</button>
								{/each}
							</div>
						{/if}

						<div class="relative">
							<button
								class="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
								onclick={() => { showLabelPicker = !showLabelPicker; labelSearch = ''; }}
							>
								<TagIcon class="h-3.5 w-3.5" />
								{localLabels.length > 0 ? $t('task.editLabels') : $t('task.addLabels')}
							</button>

							{#if showLabelPicker}
								<div class="absolute left-0 top-full z-10 mt-1 w-52 rounded-lg border border-border bg-popover shadow-xl">
									<div class="p-2">
										<input
											bind:value={labelSearch}
											type="text"
											placeholder={$t('task.searchLabels')}
											class="w-full rounded-md border border-border/50 bg-transparent px-2.5 py-1.5 text-[12px] text-foreground placeholder:text-muted-foreground/40 focus:border-border focus:outline-none"
										/>
									</div>
									<div class="max-h-48 overflow-y-auto px-1 pb-1">
										{#each filteredLabels as label (label.id)}
											<button
												class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12px] text-foreground transition-colors hover:bg-accent"
												onclick={() => toggleLabel(label.name)}
											>
												<div
													class="flex h-4 w-4 items-center justify-center rounded border border-border/50
														{localLabels.includes(label.name) ? 'border-primary bg-primary' : ''}"
												>
													{#if localLabels.includes(label.name)}
														<CheckIcon class="h-3 w-3 text-primary-foreground" strokeWidth={3} />
													{/if}
												</div>
												{label.name}
											</button>
										{/each}
										{#if filteredLabels.length === 0}
											<p class="px-2.5 py-2 text-[12px] text-muted-foreground">{$t('task.noLabelsFound')}</p>
										{/if}
									</div>
								</div>
							{/if}
						</div>
					</div>
				</div>
			</div>
	{/snippet}

	{#if fullPage}
		<div class="flex h-full flex-col">
			{@render panelContent()}
		</div>
	{:else}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="fixed inset-0 z-50 flex justify-end bg-black/50 backdrop-blur-sm"
			onclick={handleBackdropClick}
		>
			<div
				class="flex h-full w-full flex-col bg-background shadow-2xl
					md:max-w-3xl md:border-l md:border-border"
				style="animation: slideInRight 200ms ease-out"
			>
				{@render panelContent()}
			</div>
		</div>
	{/if}

	{#if showDeleteConfirm}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div
			class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
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
				<p class="mt-2 text-sm text-muted-foreground">
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
{/if}

<style>
	@keyframes slideInRight {
		from {
			transform: translateX(100%);
		}
		to {
			transform: translateX(0);
		}
	}
</style>
