<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import { t } from 'svelte-intl-precompile';

	let { tasks, searchQuery = '', completed = false, contextName = '', onResetContext }: { tasks: Task[]; searchQuery?: string; completed?: boolean; contextName?: string; onResetContext?: () => void } = $props();

	type DateGroup = { dateKey: string; label: string; tasks: Task[] };

	const completedGroups = $derived.by<DateGroup[]>(() => {
		if (!completed) return [];

		function localDateKey(d: Date): string {
			return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
		}

		const today = new Date();
		const yesterday = new Date(today);
		yesterday.setDate(yesterday.getDate() - 1);
		const todayStr = localDateKey(today);
		const yesterdayStr = localDateKey(yesterday);

		const grouped = new Map<string, Task[]>();
		for (const task of tasks) {
			const dateKey = task.completed_at ? localDateKey(new Date(task.completed_at)) : 'unknown';
			if (!grouped.has(dateKey)) grouped.set(dateKey, []);
			grouped.get(dateKey)!.push(task);
		}

		const result: DateGroup[] = [];
		for (const [dateKey, groupTasks] of grouped) {
			let label: string;
			if (dateKey === todayStr) {
				label = $t('tasks.dateToday');
			} else if (dateKey === yesterdayStr) {
				label = $t('tasks.dateYesterday');
			} else if (dateKey === 'unknown') {
				label = '—';
			} else {
				const d = new Date(dateKey + 'T00:00:00');
				label = d.toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' });
			}
			result.push({ dateKey, label, tasks: groupTasks });
		}

		return result;
	});
</script>

{#if tasks.length === 0}
	<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
		<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
		<p class="text-sm">{$t('tasks.noTasks')}</p>
		{#if contextName}
			<p class="mt-2 text-xs text-muted-foreground/60">
				{$t('tasks.context', { values: { name: contextName } })}
				{#if onResetContext}
					<span class="mx-1">·</span>
					<button class="text-muted-foreground/60 underline underline-offset-2 transition-colors hover:text-muted-foreground" onclick={onResetContext}>{$t('tasks.reset')}</button>
				{/if}
			</p>
		{/if}
	</div>
{:else if completed}
	<div class="space-y-px px-1">
		{#each completedGroups as group (group.dateKey)}
			<div class="flex items-center gap-3 px-2 pt-4 pb-1 md:px-3">
				<div class="h-px flex-1 bg-border/40"></div>
				<span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/40">{group.label}</span>
				<div class="h-px flex-1 bg-border/40"></div>
			</div>
			{#each group.tasks as task, i (task.id)}
				<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
					<TaskItem {task} {searchQuery} {completed} />
				</div>
			{/each}
		{/each}
	</div>
{:else}
	<div class="space-y-px px-1">
		{#each tasks as task, i (task.id)}
			<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
				<TaskItem {task} {searchQuery} {completed} />
			</div>
		{/each}
	</div>
{/if}
