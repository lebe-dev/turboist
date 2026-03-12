<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { completeTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks';

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
	<div class="mt-5 first:mt-0">
		<h3 class="px-2 py-1 text-sm font-semibold text-foreground/70">
			{task.content}
		</h3>
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
		<div class="group flex items-start gap-2.5 rounded-md px-2 py-2 text-sm hover:bg-accent/40 md:py-1.5">
			<button
				class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-border/60 transition-colors hover:border-primary hover:bg-primary/10 disabled:opacity-40 md:h-[18px] md:w-[18px]"
				style="-webkit-tap-highlight-color: transparent;"
				onclick={handleComplete}
				disabled={completing}
				aria-label="Завершить задачу"
			>
				{#if completing}
					<div
						class="h-2.5 w-2.5 animate-spin rounded-full border border-primary border-t-transparent"
					></div>
				{/if}
			</button>

			<div class="min-w-0 flex-1">
				<span class="break-words leading-snug text-foreground">{task.content}</span>
				{#if task.labels.length > 0 || task.due || task.sub_task_count > 0}
					<div class="mt-1 flex flex-wrap items-center gap-1.5">
						{#each task.labels as label (label)}
							<span class="rounded bg-accent px-1.5 py-0.5 text-xs text-muted-foreground"
								>{label}</span
							>
						{/each}
						{#if dueLabel}
							<span
								class="text-xs {isOverdue
									? 'text-destructive'
									: 'text-muted-foreground'}">{dueLabel}</span
							>
						{/if}
						{#if task.sub_task_count > 0}
							<span class="text-xs text-muted-foreground"
								>{task.completed_sub_task_count}/{task.sub_task_count}</span
							>
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
