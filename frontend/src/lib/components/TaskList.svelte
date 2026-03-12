<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';
	import InboxIcon from '@lucide/svelte/icons/inbox';

	let { tasks }: { tasks: Task[] } = $props();
</script>

{#if tasks.length === 0}
	<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
		<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
		<p class="text-sm">Нет задач</p>
	</div>
{:else}
	<div class="space-y-px px-1">
		{#each tasks as task, i (task.id)}
			<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
				<TaskItem {task} />
			</div>
		{/each}
	</div>
{/if}
