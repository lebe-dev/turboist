<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';
	import InboxIcon from '@lucide/svelte/icons/inbox';

	let { tasks, searchQuery = '', onselect, completed = false, contextName = '', onResetContext }: { tasks: Task[]; searchQuery?: string; onselect?: (id: string) => void; completed?: boolean; contextName?: string; onResetContext?: () => void } = $props();
</script>

{#if tasks.length === 0}
	<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
		<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
		<p class="text-sm">Нет задач</p>
		{#if contextName}
			<p class="mt-2 text-xs text-muted-foreground/60">
				Контекст: {contextName}
				{#if onResetContext}
					<span class="mx-1">·</span>
					<button class="text-muted-foreground/60 underline underline-offset-2 transition-colors hover:text-muted-foreground" onclick={onResetContext}>сбросить</button>
				{/if}
			</p>
		{/if}
	</div>
{:else}
	<div class="space-y-px px-1">
		{#each tasks as task, i (task.id)}
			<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
				<TaskItem {task} {searchQuery} {onselect} {completed} />
			</div>
		{/each}
	</div>
{/if}
