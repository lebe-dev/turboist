<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { completeTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CalendarIcon from '@lucide/svelte/icons/calendar';

	let { task, depth = 0 }: { task: Task; depth?: number } = $props();

	let completing = $state(false);

	async function handleComplete() {
		if (completing) return;
		completing = true;
		try {
			await completeTask(task.id);
			tasksStore.refresh();
		} catch (e) {
			console.error('Failed to complete task', e);
		} finally {
			completing = false;
		}
	}

	const dueLabel = $derived.by(() => {
		if (!task.due) return null;
		const d = new Date(task.due.date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const tomorrow = new Date(today);
		tomorrow.setDate(tomorrow.getDate() + 1);
		if (d.getTime() === today.getTime()) return 'Сегодня';
		if (d.getTime() === tomorrow.getTime()) return 'Завтра';
		return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
	});

	const isOverdue = $derived.by(() => {
		if (!task.due) return false;
		const d = new Date(task.due.date + 'T00:00:00');
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		return d < today;
	});
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
					<svelte:self task={child} depth={0} />
				{/each}
			</div>
		{/if}
	</div>
{:else}
	<div style="padding-left: {depth * 20}px">
		<div
			class="group flex items-start gap-3 rounded-lg px-3 py-2 transition-colors duration-150 hover:bg-accent/50"
			class:opacity-40={completing}
			class:scale-[0.99]={completing}
		>
			<button
				class="mt-0.5 flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] transition-all duration-150
					{completing
					? 'border-primary bg-primary'
					: 'border-muted-foreground/25 hover:border-primary hover:bg-primary/10'}"
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

			<div class="min-w-0 flex-1">
				<span class="break-words text-[13px] leading-relaxed text-foreground/90">{task.content}</span>
				{#if task.labels.length > 0 || task.due || task.sub_task_count > 0}
					<div class="mt-1 flex flex-wrap items-center gap-1.5">
						{#each task.labels as label (label)}
							<span class="rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] font-medium text-primary/80">{label}</span>
						{/each}
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
						{#if task.sub_task_count > 0}
							<span class="text-[11px] tabular-nums text-muted-foreground">
								{task.completed_sub_task_count}/{task.sub_task_count}
							</span>
						{/if}
					</div>
				{/if}
			</div>
		</div>

		{#if task.children && task.children.length > 0}
			<div>
				{#each task.children as child (child.id)}
					<svelte:self task={child} depth={depth + 1} />
				{/each}
			</div>
		{/if}
	</div>
{/if}
