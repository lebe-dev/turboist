<script lang="ts">
	import type { Task, Label } from '$lib/api/types';
	import { updateTask, createTask, completeTask, getLabels, getTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { onMount } from 'svelte';
	import XIcon from '@lucide/svelte/icons/x';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import TagIcon from '@lucide/svelte/icons/tag';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import RepeatIcon from '@lucide/svelte/icons/repeat';
	import MarkdownContent from './MarkdownContent.svelte';

	let {
		taskId,
		onclose,
		onselect
	}: {
		taskId: string;
		onclose: () => void;
		onselect?: (id: string) => void;
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
	const task = $derived(taskFromApi ?? taskFromStore);

	function updateLocal(updater: (t: Task) => Task) {
		if (taskFromApi) taskFromApi = updater(taskFromApi);
		if (task) tasksStore.updateTaskLocal(task.id, updater);
	}

	let parentTask = $state<Task | null>(null);

	$effect(() => {
		const pid = task?.parent_id ?? null;
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

	// --- Title editing ---
	let editingTitle = $state(false);
	let titleValue = $state('');
	let titleInput: HTMLInputElement | undefined = $state();

	function startEditTitle() {
		if (!task) return;
		titleValue = task.content;
		editingTitle = true;
		requestAnimationFrame(() => titleInput?.focus());
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
	let editingDate = $state(false);
	let dateInput: HTMLInputElement | undefined = $state();

	function startEditDate() {
		editingDate = true;
		requestAnimationFrame(() => {
			dateInput?.showPicker?.();
			dateInput?.focus();
		});
	}

	async function saveDate(e: Event) {
		if (!task) return;
		editingDate = false;
		const value = (e.target as HTMLInputElement).value;
		if (!value) return;
		const currentDate = task.due?.date ?? '';
		if (value === currentDate) return;
		// Optimistic: update local due date immediately
		updateLocal((t) => ({ ...t, due: { date: value, recurring: t.due?.recurring ?? false } }));
		try {
			await updateTask(task.id, { due_date: value });
		} catch (e) {
			console.error('Failed to update due date', e);
		}
		tasksStore.refresh();
	}

	async function clearDate() {
		if (!task || !task.due) return;
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

	// Sync local labels from store when task updates (skip during pending API call)
	$effect(() => {
		if (task && !labelsSyncing) {
			localLabels = [...task.labels];
		}
	});

	onMount(async () => {
		try {
			allLabels = await getLabels();
		} catch {
			// ignore
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

	async function handleComplete(id?: string) {
		const targetId = id ?? task?.id;
		if (!targetId || completing) return;
		completing = true;
		// Optimistic: short delay for animation, then remove locally
		await new Promise((r) => setTimeout(r, 200));
		if (targetId === task?.id) {
			// Completing the main task — remove and close panel
			tasksStore.removeTaskLocal(targetId);
		} else {
			// Completing a subtask — remove from children
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
			tasksStore.refresh();
		}
	}

	// --- Subtask priority (optimistic) ---
	async function setSubtaskPriority(childId: string, value: number) {
		if (!task) return;
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

	// --- Add sub-task ---
	let showSubtaskForm = $state(false);
	let subtaskContent = $state('');
	let subtaskInput: HTMLInputElement | undefined = $state();
	let addingSubtask = $state(false);

	function startAddSubtask() {
		subtaskContent = '';
		showSubtaskForm = true;
		requestAnimationFrame(() => subtaskInput?.focus());
	}

	async function saveSubtask() {
		if (!task || !subtaskContent.trim() || addingSubtask) return;
		addingSubtask = true;
		const content = subtaskContent.trim();
		const labels = [...task.labels];
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
		if (d.getTime() === today.getTime()) return 'Today';
		if (d.getTime() === tomorrow.getTime()) return 'Tomorrow';
		return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' });
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
	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (showLabelPicker) {
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
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onclose();
		}
	}

	const collapsed = $derived(task ? collapsedStore.isCollapsed(task.id) : false);
</script>

{#if task}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex justify-end bg-black/50 backdrop-blur-sm"
		onkeydown={handleKeydown}
		onclick={handleBackdropClick}
	>
		<div
			class="flex h-full w-full flex-col bg-background shadow-2xl
				md:max-w-3xl md:border-l md:border-border"
			style="animation: slideInRight 200ms ease-out"
		>
			<!-- Header -->
			<div class="flex shrink-0 items-center justify-between border-b border-border/50 px-5 py-3">
				<div class="flex items-center gap-2 text-[12px] text-muted-foreground">
					{#if task.parent_id && parentTask}
						<button
							class="flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:bg-accent hover:text-foreground"
							onclick={() => onselect?.(parentTask!.id)}
						>
							<ChevronRightIcon class="h-3 w-3 rotate-180" />
							{parentTask.content}
						</button>
					{:else if task.sub_task_count > 0}
						<span>{task.completed_sub_task_count}/{task.sub_task_count} subtasks</span>
					{/if}
				</div>
				<button
					class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					onclick={onclose}
					aria-label="Close"
				>
					<XIcon class="h-4 w-4" />
				</button>
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
								<input
									bind:this={titleInput}
									bind:value={titleValue}
									type="text"
									class="w-full bg-transparent text-lg font-semibold text-foreground focus:outline-none"
									onkeydown={(e) => {
										if (e.key === 'Enter') {
											e.preventDefault();
											saveTitle();
										}
									}}
								/>
								<div class="mt-2 flex items-center gap-2">
									<button
										class="rounded-md bg-primary px-3 py-1 text-[12px] font-medium text-primary-foreground transition-colors hover:bg-primary/90"
										onclick={saveTitle}
									>Save</button>
									<button
										class="rounded-md px-3 py-1 text-[12px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
										onclick={cancelTitle}
									>Cancel</button>
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
								placeholder="Add description..."
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
								>Save</button>
								<button
									class="rounded-md px-3 py-1 text-[12px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
									onclick={cancelDesc}
								>Cancel</button>
							</div>
						{:else}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								class="cursor-text rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent/50
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
						<div class="mt-6 pl-8">
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
										<div class="group flex items-start gap-2.5 rounded-lg px-2 py-1.5 transition-colors hover:bg-accent/50">
											<button
												class="mt-0.5 flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
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
													{#if child.priority > 1}
														<!-- svelte-ignore a11y_click_events_have_key_events -->
														<!-- svelte-ignore a11y_no_static_element_interactions -->
														<span
															class="cursor-pointer rounded px-1 py-0.5 text-[10px] font-medium transition-colors
																{child.priority === 4 ? 'text-red-500' : child.priority === 3 ? 'text-amber-500' : 'text-blue-400'}"
															role="button"
															tabindex="-1"
															onclick={(e) => { e.stopPropagation(); setSubtaskPriority(child.id, child.priority === 4 ? 3 : child.priority === 3 ? 2 : 1); }}
														>{child.priority === 4 ? 'P1' : child.priority === 3 ? 'P2' : 'P3'}</span>
													{/if}
													{#each child.labels as label (label)}
														<span class="rounded px-1.5 py-0.5 text-[11px] bg-muted text-muted-foreground">{label}</span>
													{/each}
												</div>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}

					<!-- Add sub-task -->
					<div class="mt-4 pl-8">
						{#if showSubtaskForm}
							<div class="flex items-center gap-2">
								<div class="flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-full border-[1.5px] border-muted-foreground/25"></div>
								<input
									bind:this={subtaskInput}
									bind:value={subtaskContent}
									type="text"
									class="flex-1 bg-transparent text-[13px] text-foreground placeholder:text-muted-foreground/40 focus:outline-none"
									placeholder="Sub-task name"
									disabled={addingSubtask}
									onkeydown={(e) => {
										if (e.key === 'Enter') {
											e.preventDefault();
											saveSubtask();
										}
										if (e.key === 'Escape') {
											showSubtaskForm = false;
										}
									}}
									onblur={() => {
										if (!subtaskContent.trim()) showSubtaskForm = false;
									}}
								/>
								{#if subtaskContent.trim()}
									<button
										class="rounded-md bg-primary px-2.5 py-1 text-[11px] font-medium text-primary-foreground transition-colors hover:bg-primary/90"
										onclick={saveSubtask}
										disabled={addingSubtask}
									>
										{addingSubtask ? '...' : 'Add'}
									</button>
								{/if}
							</div>
						{:else}
							<button
								class="flex items-center gap-2 text-[13px] text-muted-foreground transition-colors hover:text-primary"
								onclick={startAddSubtask}
							>
								<PlusIcon class="h-4 w-4" />
								Add sub-task
							</button>
						{/if}
					</div>
				</div>

				<!-- Right: sidebar -->
				<div class="hidden w-60 shrink-0 space-y-5 overflow-y-auto border-l border-border/50 p-5 md:block">
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
							<div class="relative">
								<button
									class="flex items-center justify-center rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
									onclick={startEditDate}
									aria-label="Pick custom date"
								>
									<CalendarIcon class="h-3.5 w-3.5" />
								</button>
								{#if editingDate}
									<input
										bind:this={dateInput}
										type="date"
										value={task.due?.date ?? ''}
										class="absolute left-0 top-full z-10 mt-1 w-44 rounded-md border border-border/50 bg-popover px-2.5 py-1.5 text-[13px] text-foreground shadow-lg focus:border-border focus:outline-none"
										onchange={saveDate}
										onblur={() => (editingDate = false)}
									/>
								{/if}
							</div>
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
					</div>

					<!-- Priority -->
					<div>
						<h3 class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">Priority</h3>
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
						<h3 class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">Labels</h3>
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
								{localLabels.length > 0 ? 'Edit labels' : 'Add labels'}
							</button>

							{#if showLabelPicker}
								<div class="absolute left-0 top-full z-10 mt-1 w-52 rounded-lg border border-border bg-popover shadow-xl">
									<div class="p-2">
										<input
											bind:value={labelSearch}
											type="text"
											placeholder="Search labels..."
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
											<p class="px-2.5 py-2 text-[12px] text-muted-foreground">No labels found</p>
										{/if}
									</div>
								</div>
							{/if}
						</div>
					</div>
				</div>
			</div>
		</div>
	</div>
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
