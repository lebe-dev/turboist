<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { completeTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import PinIcon from '@lucide/svelte/icons/pin';

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
				<span class="break-words text-[13px] leading-relaxed {completed ? 'line-through text-muted-foreground' : 'text-foreground/90'}">{task.content}</span>
				{#if task.description && !completed}
					<p class="truncate text-[12px] text-muted-foreground">{task.description}</p>
				{/if}
				{#if completed && completedAtLabel}
					<p class="text-[11px] text-muted-foreground/60">{completedAtLabel}</p>
				{:else if task.labels.length > 0 || task.due || task.sub_task_count > 0}
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
						{#each task.labels as label (label)}
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

			{#if !completed && canPin}
				<button
					class="absolute right-2 top-2 flex h-5 w-5 items-center justify-center rounded transition-all duration-150
						{isPinned
						? 'text-primary opacity-60 hover:opacity-100'
						: 'text-muted-foreground/40 opacity-0 group-hover:opacity-100 hover:text-muted-foreground'}"
					onclick={handlePin}
					aria-label={isPinned ? 'Unpin task' : 'Pin task'}
				>
					<PinIcon class="h-3 w-3" />
				</button>
			{/if}
		</div>

		{#if hasChildren && !collapsed && !completed}
			<div>
				{#each task.children as child (child.id)}
					<svelte:self task={child} depth={depth + 1} {searchQuery} {onselect} {dimmed} {hideTodayDue} {hideTomorrowDue} />
				{/each}
			</div>
		{/if}
	</div>
{/if}
