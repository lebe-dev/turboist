<script lang="ts">
	import type { Task, Label } from '$lib/api/types';
	import { updateTask, createTask, completeTask, deleteTask, duplicateTask, getTask, getCompletedSubtasks } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import { nextActionStore } from '$lib/stores/next-action.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { toast } from 'svelte-sonner';
	import { logger } from '$lib/stores/logger';
	import { goto } from '$app/navigation';
	import { tick } from 'svelte';
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
	import CopyIcon from '@lucide/svelte/icons/copy';
	import PinIcon from '@lucide/svelte/icons/pin';
	import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import SunIcon from '@lucide/svelte/icons/sun';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import MarkdownContent from './MarkdownContent.svelte';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import TaskDropdownMenu from './TaskDropdownMenu.svelte';
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

	function saveTitle() {
		if (!task || !editingTitle) return;
		editingTitle = false;
		const trimmed = titleValue.trim();
		if (!trimmed || trimmed === task.content) return;
		const taskId = task.id;
		updateLocal((t) => ({ ...t, content: trimmed }));
		updateTask(taskId, { content: trimmed }).catch((e) => {
			logger.error('tasks', `update title failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
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

	function saveDesc() {
		if (!task || !editingDesc) return;
		editingDesc = false;
		const trimmed = descValue.trim();
		if (trimmed === task.description) return;
		const taskId = task.id;
		updateLocal((t) => ({ ...t, description: trimmed }));
		updateTask(taskId, { description: trimmed }).catch((e) => {
			logger.error('tasks', `update description failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	function cancelDesc() {
		editingDesc = false;
	}

	// --- Priority (optimistic) ---
	let showPriorityPicker = $state(false);
	let labelPickerRef: HTMLDivElement | undefined = $state();
	let priorityPickerRef: HTMLDivElement | undefined = $state();
	let calendarRef: HTMLDivElement | undefined = $state();
	let titleEditRef: HTMLDivElement | undefined = $state();
	let descEditRef: HTMLDivElement | undefined = $state();

	// Close pickers/editors on click outside
	$effect(() => {
		if (!showLabelPicker && !showPriorityPicker && !showCalendar && !editingTitle && !editingDesc) return;

		function handlePointerDown(e: PointerEvent) {
			const target = e.target as Node;
			if (showLabelPicker && labelPickerRef && !labelPickerRef.contains(target)) {
				showLabelPicker = false;
			}
			if (showPriorityPicker && priorityPickerRef && !priorityPickerRef.contains(target)) {
				showPriorityPicker = false;
			}
			if (showCalendar && calendarRef && !calendarRef.contains(target)) {
				showCalendar = false;
			}
			if (editingTitle && titleEditRef && !titleEditRef.contains(target)) {
				saveTitle();
			}
			if (editingDesc && descEditRef && !descEditRef.contains(target)) {
				saveDesc();
			}
		}

		const frame = requestAnimationFrame(() => {
			document.addEventListener('pointerdown', handlePointerDown);
		});
		return () => {
			cancelAnimationFrame(frame);
			document.removeEventListener('pointerdown', handlePointerDown);
		};
	});
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

	function setPriority(value: number) {
		if (!task) return;
		dropdownOpen = false;
		showPriorityPicker = false;
		if (value === localPriority) return;
		const taskId = task.id;
		localPriority = value;
		prioritySyncing = true;
		updateLocal((t) => ({ ...t, priority: value }));
		updateTask(taskId, { priority: value }).catch((e) => {
			if (task) localPriority = task.priority;
			logger.error('tasks', `update priority failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		}).finally(() => {
			prioritySyncing = false;
		});
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

	function onCalendarSelect(value: DateValue | undefined) {
		if (!task || !value) return;
		const dateStr = `${value.year}-${String(value.month).padStart(2, '0')}-${String(value.day).padStart(2, '0')}`;
		const currentDate = task.due?.date ?? '';
		if (dateStr === currentDate) {
			showCalendar = false;
			return;
		}
		showCalendar = false;
		const taskId = task.id;
		updateLocal((t) => ({ ...t, due: { date: dateStr, recurring: t.due?.recurring ?? false } }));
		// Don't removeTaskLocal here — let WS handle list removal so the panel stays open
		updateTask(taskId, { due_date: dateStr }).catch((e) => {
			logger.error('tasks', `update due date failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	function clearDate() {
		if (!task || !task.due) return;
		dropdownOpen = false;
		const taskId = task.id;
		updateLocal((t) => ({ ...t, due: null }));
		// Don't removeTaskLocal here — let WS handle list removal so the panel stays open
		updateTask(taskId, { due_date: '' }).catch((e) => {
			logger.error('tasks', `clear due date failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
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

function setDateQuick(date: string) {
		if (!task) return;
		dropdownOpen = false;
		const currentDate = task.due?.date ?? '';
		if (date === currentDate) return;
		const taskId = task.id;
		updateLocal((t) => ({ ...t, due: { date, recurring: false } }));
		// Don't removeTaskLocal here — let WS handle list removal so the panel stays open
		updateTask(taskId, { due_date: date }).catch((e) => {
			logger.error('tasks', `set due date failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	// --- Labels (optimistic) ---
	const allLabels = $derived(appStore.labels);
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

	function toggleLabel(name: string) {
		if (!task) return;
		const newLabels = localLabels.includes(name)
			? localLabels.filter((l) => l !== name)
			: [...localLabels, name];
		const taskId = task.id;
		localLabels = newLabels;
		labelsSyncing = true;
		updateLocal((t) => ({ ...t, labels: newLabels }));
		updateTask(taskId, { labels: newLabels }).catch((e) => {
			if (task) localLabels = [...task.labels];
			logger.error('tasks', `update labels failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		}).finally(() => {
			labelsSyncing = false;
		});
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
		completeTask(targetId).catch((e) => {
			logger.error('tasks', `complete failed: ${e}`);
			toast.error($t('errors.completeFailed'));
			tasksStore.clearPendingRemoval(targetId);
			tasksStore.refresh();
		});
	}

	// --- Subtask menu ---
	let openSubtaskMenuId = $state<string | null>(null);

	// --- Subtask priority (optimistic) ---
	function setSubtaskPriority(childId: string, value: number) {
		if (!task) return;
		openSubtaskMenuId = null;
		updateLocal((t) => ({
			...t,
			children: t.children
				.map((c) => (c.id === childId ? { ...c, priority: value } : c))
				.sort((a, b) => b.priority - a.priority)
		}));
		updateTask(childId, { priority: value }).catch((e) => {
			logger.error('tasks', `update subtask priority failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	// --- Subtask date ---
	function setSubtaskDate(childId: string, date: string) {
		openSubtaskMenuId = null;
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) =>
				c.id === childId ? { ...c, due: { date, recurring: false } } : c
			)
		}));
		updateTask(childId, { due_date: date }).catch((e) => {
			logger.error('tasks', `set subtask date failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	function clearSubtaskDate(childId: string) {
		openSubtaskMenuId = null;
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) =>
				c.id === childId ? { ...c, due: null } : c
			)
		}));
		updateTask(childId, { due_date: '' }).catch((e) => {
			logger.error('tasks', `clear subtask date failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	let subtaskCalendarTargetId = $state<string | null>(null);

	function openSubtaskDatePicker(childId: string) {
		subtaskCalendarTargetId = subtaskCalendarTargetId === childId ? null : childId;
	}

	function onSubtaskCalendarSelect(value: DateValue | undefined) {
		if (!value || !subtaskCalendarTargetId) return;
		const dateStr = `${value.year}-${String(value.month).padStart(2, '0')}-${String(value.day).padStart(2, '0')}`;
		const targetId = subtaskCalendarTargetId;
		subtaskCalendarTargetId = null;
		setSubtaskDate(targetId, dateStr);
	}

	// --- Delete subtask ---
	function deleteSubtask(childId: string) {
		if (!task) return;
		updateLocal((t) => ({
			...t,
			children: t.children.filter((c) => c.id !== childId),
			sub_task_count: Math.max(0, t.sub_task_count - 1)
		}));
		deleteTask(childId).catch((e) => {
			logger.error('tasks', `delete subtask failed: ${e}`);
			toast.error($t('errors.deleteFailed'));
			tasksStore.refresh();
		});
	}

	// --- Duplicate subtask ---
	function duplicateSubtask(childId: string) {
		openSubtaskMenuId = null;
		duplicateTask(childId).then(() => {
			getTask(taskId).then((t) => { taskFromApi = t; }).catch(() => {});
		}).catch((e) => {
			logger.error('tasks', `duplicate subtask failed: ${e}`);
			if (e instanceof Error && e.message === 'offline:not-queueable') {
				toast.error($t('pwa.requiresNetwork'));
			} else {
				toast.error($t('errors.duplicateFailed'));
			}
			tasksStore.refresh();
		});
	}

	// --- Add sub-task ---
	let showSubtaskForm = $state(false);
	let subtaskContent = $state('');
	let subtaskTextarea: HTMLTextAreaElement | undefined = $state();
	let addingSubtask = $state(false);

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
		tick().then(() => {
			subtaskTextarea?.focus();
			subtaskTextarea?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
		});
	}

	function saveSubtask() {
		if (!task || !subtaskContent.trim() || addingSubtask) return;
		const content = subtaskContent.trim();
		const inheritableParentLabels = task.labels.filter((l) => appStore.shouldInheritToSubtasks(l));
		const labels = [...new Set([...inheritableParentLabels, ...contextLabels])];
		const parentId = task.id;
		const tempId = `temp-${Date.now()}`;
		const optimistic: Task = {
			id: tempId,
			content,
			description: '',
			project_id: task.project_id,
			section_id: task.section_id,
			parent_id: parentId,
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
		// Keep form open for adding more subtasks; reset to detected prefix
		const nextPrefix = detectPrefixFromSiblings([...(task?.children ?? []), optimistic]);
		subtaskContent = nextPrefix || extractPrefix(task?.content ?? '');
		tick().then(() => {
			subtaskTextarea?.focus();
			subtaskTextarea?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
		});
		createTask(
			{ content, description: '', labels, priority: 1, parent_id: parentId },
			contextsStore.activeContextId ?? undefined
		).then(() => {
			// Re-fetch to replace temp child IDs with real ones
			getTask(taskId).then((t) => { taskFromApi = t; }).catch(() => {});
		}).catch((e) => {
			logger.error('tasks', `create subtask failed: ${e}`);
			toast.error($t('errors.createFailed'));
			tasksStore.refresh();
		});
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

	function handleDuplicate() {
		if (!task || duplicating) return;
		duplicating = true;
		dropdownOpen = false;
		const taskContent = task.content;
		const sourceId = task.id;
		const tempId = `temp-dup-${Date.now()}`;
		const clone: Task = {
			...task,
			id: tempId,
			children: [],
			sub_task_count: 0,
			completed_sub_task_count: 0,
		};
		tasksStore.insertAfterLocal(sourceId, clone);
		duplicateTask(sourceId).then((newId) => {
			toast.dismiss();
			toast(`Duplicated: ${taskContent}`, {
				duration: 5000,
				action: {
					label: 'Open',
					onClick: () => goto(`/task/${newId}`)
				}
			});
		}).catch((e) => {
			logger.error('tasks', `duplicate failed: ${e}`);
			tasksStore.removeTaskLocal(tempId);
			if (e instanceof Error && e.message === 'offline:not-queueable') {
				toast.error($t('pwa.requiresNetwork'));
			} else {
				toast.error($t('errors.duplicateFailed'));
			}
		}).finally(() => {
			duplicating = false;
		});
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

	// --- Bulk operations (subtasks only) ---
	function resetSubtaskPriorities() {
		if (!task) return;
		dropdownOpen = false;
		const children = task.children;
		if (children.length === 0) return;
		const toReset = children.filter((c) => c.priority !== 1);
		if (toReset.length === 0) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) => toReset.some((r) => r.id === c.id) ? { ...c, priority: 1 } : c)
		}));
		Promise.all(toReset.map((c) => updateTask(c.id, { priority: 1 }))).catch((e) => {
			logger.error('tasks', `reset subtask priorities failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	function resetSubtaskLabels() {
		if (!task) return;
		dropdownOpen = false;
		const children = task.children;
		if (children.length === 0) return;
		const toReset = children.filter((c) => c.labels.length > 0);
		if (toReset.length === 0) return;
		updateLocal((t) => ({
			...t,
			children: t.children.map((c) => toReset.some((r) => r.id === c.id) ? { ...c, labels: [] } : c)
		}));
		Promise.all(toReset.map((c) => updateTask(c.id, { labels: [] }))).catch((e) => {
			logger.error('tasks', `reset subtask labels failed: ${e}`);
			toast.error($t('errors.updateFailed'));
			tasksStore.refresh();
		});
	}

	// --- Delete task ---
	let showDeleteConfirm = $state(false);
	function handleDelete() {
		if (!task) return;
		const taskId = task.id;
		tasksStore.removeTaskLocal(taskId);
		showDeleteConfirm = false;
		onclose();
		deleteTask(taskId).catch((e) => {
			logger.error('tasks', `delete failed: ${e}`);
			toast.error($t('errors.deleteFailed'));
			tasksStore.clearPendingRemoval(taskId);
			tasksStore.refresh();
		});
	}

	function focusOnMount(node: HTMLElement) {
		requestAnimationFrame(() => node.focus());
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if task}
	{#snippet subtaskFormContent()}
		<textarea
			bind:this={subtaskTextarea}
			bind:value={subtaskContent}
			placeholder={$t('task.subtaskName')}
			rows="1"
			class="w-full resize-none bg-transparent text-[13px] leading-snug text-foreground placeholder:text-muted-foreground/40 focus:outline-none"
			disabled={addingSubtask}
			oninput={(e) => {
				const target = e.currentTarget;
				target.style.height = 'auto';
				target.style.height = target.scrollHeight + 'px';
			}}
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
		<div class="mt-1.5 flex items-center justify-end gap-2">
			<button
				class="rounded-md px-3 py-1 text-[12px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={() => { showSubtaskForm = false; subtaskContent = ''; }}
			>
				{$t('dialog.cancel')}
			</button>
			<button
				class="rounded-md px-3 py-1 text-[12px] font-medium transition-colors
					{subtaskContent.trim()
						? 'bg-primary text-primary-foreground hover:bg-primary/90'
						: 'bg-muted text-muted-foreground cursor-not-allowed'}"
				disabled={!subtaskContent.trim() || addingSubtask}
				onclick={saveSubtask}
			>
				{addingSubtask ? '...' : $t('task.add')}
			</button>
		</div>
	{/snippet}

	{#snippet panelContent()}
		<!-- Header -->
		<div class="flex shrink-0 items-center justify-between border-b border-border/50 px-5 py-3">
			<div class="flex items-center gap-2 text-[12px] text-muted-foreground">
				{#if fullPage}
					<button
						class="flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						onclick={onclose}
					>
						<ArrowLeftIcon class="h-3.5 w-3.5" />
					</button>
					{#if task.parent_id && parentTask}
						<button
							class="flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground min-w-0"
							onclick={() => onselect?.(parentTask!.id)}
						>
							<span class="truncate">{parentTask.content}</span>
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
				<TaskDropdownMenu
					bind:open={dropdownOpen}
					{task}
					onDuplicate={handleDuplicate}
					onCopy={() => { if (task) navigator.clipboard.writeText(task.content); dropdownOpen = false; }}
					{canPin}
					{isPinned}
					onPin={handlePin}
					subtaskCount={task.children.length}
					onResetSubtaskPriorities={resetSubtaskPriorities}
					onResetSubtaskLabels={resetSubtaskLabels}
					onSetDate={setDateQuick}
					onClearDate={clearDate}
					onSetPriority={setPriority}
					onDelete={() => { dropdownOpen = false; showDeleteConfirm = true; }}
				>
					{#snippet trigger()}
						<DropdownMenu.Trigger
							class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						>
							<EllipsisVerticalIcon class="h-4 w-4" />
						</DropdownMenu.Trigger>
					{/snippet}
				</TaskDropdownMenu>
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
							<div bind:this={titleEditRef} class="flex-1">
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
							<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
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
							<div bind:this={descEditRef}>
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

					<!-- Mobile metadata (date, priority, labels) -->
					<div class="mt-5 space-y-4 pl-8 md:hidden">
						<!-- Date -->
						<div bind:this={calendarRef}>
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
							{#if task.due && task.due.date !== todayDateStr() && task.due.date !== tomorrowDateStr()}
								<p class="mt-1.5 text-[12px] font-medium {isOverdue(task.due.date) ? 'text-destructive' : 'text-muted-foreground'}">
									{formatDueDate(task.due.date)}
								</p>
							{/if}
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
							<div bind:this={priorityPickerRef} class="relative">
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

							<div bind:this={labelPickerRef} class="relative">
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
											<TaskDropdownMenu
												open={openSubtaskMenuId === child.id}
												onOpenChange={(v) => { openSubtaskMenuId = v ? child.id : null; }}
												task={child}
												onEdit={() => onselect?.(child.id)}
												onDuplicate={() => duplicateSubtask(child.id)}
												onCopy={() => navigator.clipboard.writeText(child.content)}
												onSetDate={(d) => setSubtaskDate(child.id, d)}
												onClearDate={() => clearSubtaskDate(child.id)}
												onOpenDatePicker={() => openSubtaskDatePicker(child.id)}
												showCalendar={subtaskCalendarTargetId === child.id}
												calendarValue={child.due?.date ? parseDate(child.due.date) : undefined}
												onCalendarSelect={onSubtaskCalendarSelect}
												onSetPriority={(p) => setSubtaskPriority(child.id, p)}
												onDelete={() => deleteSubtask(child.id)}
												width="w-64"
											>
												{#snippet trigger()}
													<DropdownMenu.Trigger
														class="absolute right-1 top-1/2 -translate-y-1/2 flex h-6 w-6 items-center justify-center rounded text-muted-foreground/40 opacity-0 transition-all duration-150 group-hover:opacity-100 hover:text-muted-foreground"
														onclick={(e: MouseEvent) => e.stopPropagation()}
													>
														<EllipsisIcon class="h-4 w-4" />
													</DropdownMenu.Trigger>
												{/snippet}
											</TaskDropdownMenu>
										</div>
									{/each}
								</div>
								{/if}
						</div>
					{/if}

					<!-- Add sub-task -->
					<div class="mt-4">
						{#if showSubtaskForm && !fullPage}
							<div class="rounded-lg border border-border/50 bg-accent/20 px-3 py-2">
								{@render subtaskFormContent()}
							</div>
						{:else if !showSubtaskForm}
							<button
								class="flex items-center gap-2 text-[13px] text-muted-foreground transition-colors hover:text-primary"
								onclick={startAddSubtask}
							>
								<PlusIcon class="h-4 w-4" />
								{$t('task.addSubtask')}
							</button>
						{/if}
					</div>

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
					<div bind:this={calendarRef}>
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
						{#if task.due && task.due.date !== todayDateStr() && task.due.date !== tomorrowDateStr()}
							<p class="mt-1.5 text-[12px] font-medium {isOverdue(task.due.date) ? 'text-destructive' : 'text-muted-foreground'}">
								{formatDueDate(task.due.date)}
							</p>
						{/if}
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
						<div bind:this={priorityPickerRef} class="relative">
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

						<div bind:this={labelPickerRef} class="relative">
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
											use:focusOnMount
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
		<!-- svelte-ignore a11y_click_events_have_key_events -->
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

	{#if showSubtaskForm && fullPage}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div
			class="fixed inset-0 z-[60] flex items-start justify-center pt-[20vh] bg-black/60 backdrop-blur-sm"
			onclick={(e) => { if (e.target === e.currentTarget) { showSubtaskForm = false; subtaskContent = ''; } }}
			onkeydown={(e) => { if (e.key === 'Escape') { showSubtaskForm = false; subtaskContent = ''; } }}
		>
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="w-full max-w-lg mx-4 rounded-xl border border-border bg-popover px-4 py-3 shadow-2xl animate-fade-in-up"
				onclick={(e) => e.stopPropagation()}
			>
				<p class="mb-2 text-[12px] font-medium text-muted-foreground">{$t('task.addSubtask')}</p>
				{@render subtaskFormContent()}
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
