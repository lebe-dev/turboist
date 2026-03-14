<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { completeTask, deleteTask, updateTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import PinIcon from '@lucide/svelte/icons/pin';
	import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';
	import TrashIcon from '@lucide/svelte/icons/trash-2';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import SunIcon from '@lucide/svelte/icons/sun';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import XIcon from '@lucide/svelte/icons/x';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import MarkdownContent from './MarkdownContent.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';

	let { task, depth = 0, searchQuery = '', onselect, dimmed = false, hideTodayDue = false, hideTomorrowDue = false, completed = false }: { task: Task; depth?: number; searchQuery?: string; onselect?: (id: string) => void; dimmed?: boolean; hideTodayDue?: boolean; hideTomorrowDue?: boolean; completed?: boolean } = $props();

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

	let completing = $state(false);

	async function handleComplete() {
		if (completing) return;
		completing = true;
		await new Promise((r) => setTimeout(r, 200));
		tasksStore.removeTaskLocal(task.id);
		completing = false;
		try {
			await completeTask(task.id);
		} catch (e) {
			console.error('Failed to complete task', e);
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
		if (d.getTime() === today.getTime()) return hideTodayDue ? null : 'Сегодня';
		if (d.getTime() === tomorrow.getTime()) return hideTomorrowDue ? null : 'Завтра';
		return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
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
		return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
	});

	const dayPartLabels = $derived(new Set(tasksStore.config?.day_parts?.map((dp) => dp.label) ?? []));
	const visibleLabels = $derived(task.labels.filter((l) => !dayPartLabels.has(l)));

	const isPinned = $derived(pinnedStore.isPinned(task.id));
	const canPin = $derived(isPinned || !pinnedStore.isFull);

	function handlePin(e: MouseEvent) {
		e.stopPropagation();
		if (isPinned) {
			pinnedStore.unpin(task.id);
		} else {
			pinnedStore.pin({ id: task.id, content: task.content });
		}
	}

	// --- Date helpers ---
	function todayStr(): string {
		const d = new Date();
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	function tomorrowStr(): string {
		const d = new Date();
		d.setDate(d.getDate() + 1);
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	const isToday = $derived(task.due?.date === todayStr());
	const isTomorrow = $derived(task.due?.date === tomorrowStr());

	async function setDate(date: string) {
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
	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500' },
		{ value: 3, label: 'P2', color: 'text-amber-500' },
		{ value: 2, label: 'P3', color: 'text-blue-400' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground' },
	];

	async function setPriority(value: number) {
		if (task.priority === value) return;
		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, priority: value }));
		try {
			await updateTask(task.id, { priority: value });
		} catch (e) {
			console.error('Failed to update priority', e);
		}
		tasksStore.refresh();
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
			tasksStore.refresh();
		} finally {
			deleting = false;
		}
	}
</script>

{#if task.is_project_task}
	<div class="mt-6 first:mt-0">
		<div class="mb-1.5 flex items-center gap-3 px-3">
			<div class="h-px flex-1 bg-border/60"></div>
			<h3 class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				{task.content}
			</h3>
			<div class="h-px flex-1 bg-border/60"></div>
		</div>
		{#if task.children.length > 0}
			<div>
				{#each task.children as child (child.id)}
					<svelte:self task={child} depth={0} {searchQuery} {onselect} />
				{/each}
			</div>
		{/if}
	</div>
{:else if visible}
	<div style="padding-left: {depth * 20}px">
		<div
			class="group relative flex items-start gap-3 rounded-lg px-3 py-2 transition-colors duration-150 hover:bg-accent/50"
			class:opacity-40={completing}
			class:scale-[0.99]={completing}
		>
			{#if completed}
				<span class="mt-0.5 flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] border-muted-foreground/30 bg-muted-foreground/10">
					<CheckIcon class="h-2.5 w-2.5 text-muted-foreground/60" strokeWidth={3} />
				</span>
			{:else}
				<button
					class="mt-0.5 flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
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

			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="min-w-0 flex-1" class:cursor-pointer={!completed} onclick={() => { if (!completed) onselect?.(task.id); }}>
				<MarkdownContent text={task.content} class="break-words text-[13px] leading-relaxed {completed ? 'line-through text-muted-foreground' : 'text-foreground/90'}" />
				{#if task.description && !completed}
					<p class="truncate text-[12px] text-muted-foreground"><MarkdownContent text={task.description} /></p>
				{/if}
				{#if completed && completedAtLabel}
					<p class="text-[11px] text-muted-foreground/60">{completedAtLabel}</p>
				{:else if visibleLabels.length > 0 || task.due || task.sub_task_count > 0}
					<div class="mt-1 flex flex-wrap items-center gap-1.5">
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
							<span class="rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">{label}</span>
						{/each}
						{#if task.sub_task_count > 0}
							<button
								class="flex items-center gap-0.5 text-[11px] tabular-nums text-muted-foreground hover:text-foreground transition-colors"
								onclick={() => collapsedStore.toggle(task.id)}
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
			</div>

			{#if !completed}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="absolute right-1 top-1.5 flex h-5 w-5 items-center justify-center rounded text-muted-foreground/40 opacity-0 transition-all duration-150 group-hover:opacity-100 hover:text-muted-foreground"
						onclick={(e: MouseEvent) => e.stopPropagation()}
					>
						<EllipsisVerticalIcon class="h-3.5 w-3.5" />
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end" class="w-52">
						<!-- Edit -->
						<DropdownMenu.Item onclick={() => onselect?.(task.id)}>
							<PencilIcon class="h-4 w-4" />
							Edit
						</DropdownMenu.Item>

						{#if canPin}
							<DropdownMenu.Item onclick={(e: MouseEvent) => handlePin(e)}>
								<PinIcon class="h-4 w-4" />
								{isPinned ? 'Unpin' : 'Pin'}
							</DropdownMenu.Item>
						{/if}

						<DropdownMenu.Separator />

						<!-- Date -->
						<div class="px-2 py-1.5">
							<p class="text-xs font-semibold text-muted-foreground">Date</p>
							<div class="mt-1.5 flex items-center gap-1">
								<button
									class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
										{isToday ? 'bg-accent text-green-500' : 'text-green-500 hover:bg-accent'}"
									onclick={() => setDate(todayStr())}
									aria-label="Today"
								>
									<CalendarIcon class="h-4 w-4" />
								</button>
								<button
									class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
										{isTomorrow ? 'bg-accent text-amber-500' : 'text-amber-500 hover:bg-accent'}"
									onclick={() => setDate(tomorrowStr())}
									aria-label="Tomorrow"
								>
									<SunIcon class="h-4 w-4" />
								</button>
								<div class="relative">
									<button
										class="flex h-7 w-7 items-center justify-center rounded-md text-purple-400 transition-colors hover:bg-accent"
										onclick={openDatePicker}
										aria-label="Pick date"
									>
										<ArrowRightIcon class="h-4 w-4" />
									</button>
									<input
										bind:this={dateInput}
										type="date"
										value={task.due?.date ?? ''}
										class="pointer-events-none absolute left-0 top-0 h-0 w-0 opacity-0"
										onchange={onDatePicked}
									/>
								</div>
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
							<p class="text-xs font-semibold text-muted-foreground">Priority</p>
							<div class="mt-1.5 flex items-center gap-1">
								{#each priorityItems as p (p.value)}
									<button
										class="flex h-7 w-7 items-center justify-center rounded-md transition-colors {p.color}
											{task.priority === p.value ? 'bg-accent' : 'hover:bg-accent'}"
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
							onclick={() => { showDeleteConfirm = true; }}
						>
							<TrashIcon class="h-4 w-4" />
							Delete
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{/if}
		</div>

		{#if showDeleteConfirm}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<div
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
					<h3 class="text-lg font-semibold text-foreground">Delete task?</h3>
					<p class="mt-2 text-sm text-muted-foreground">
						The <span class="font-medium text-foreground">{task.content}</span> task will be permanently deleted.
					</p>
					<div class="mt-4 flex justify-end gap-2">
						<button
							class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
							onclick={() => { showDeleteConfirm = false; }}
						>
							Cancel
						</button>
						<button
							class="rounded-md bg-destructive px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-destructive/90"
							onclick={handleDelete}
							disabled={deleting}
						>
							Delete
						</button>
					</div>
				</div>
			</div>
		{/if}

		{#if hasChildren && !collapsed && !completed}
			<div>
				{#each task.children as child (child.id)}
					<svelte:self task={child} depth={depth + 1} {searchQuery} {onselect} {dimmed} {hideTodayDue} {hideTomorrowDue} />
				{/each}
			</div>
		{/if}
	</div>
{/if}
