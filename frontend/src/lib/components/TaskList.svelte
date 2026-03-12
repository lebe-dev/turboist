<script lang="ts">
	import type { Task } from '$lib/api/types';
	import TaskItem from './TaskItem.svelte';

	let { tasks }: { tasks: Task[] } = $props();
</script>

{#if tasks.length === 0}
	<p class="px-4 py-12 text-center text-sm text-muted-foreground">Нет задач</p>
{:else}
	<div class="px-2 py-2">
		{#each tasks as task (task.id)}
			<TaskItem {task} />
		{/each}
	</div>
{/if}
