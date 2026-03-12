<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import TaskList from '$lib/components/TaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';

	onMount(() => {
		tasksStore.start();
	});

	onDestroy(() => {
		tasksStore.stop();
	});

	let mounted = false;

	$effect(() => {
		// Track context/view changes and refresh tasks
		contextsStore.activeContextId;
		contextsStore.activeView;
		if (mounted) tasksStore.refresh();
		mounted = true;
	});

	const viewTitles: Record<string, string> = {
		all: 'Все задачи',
		weekly: 'На неделе',
		'next-week': 'На следующей неделе'
	};

	const title = $derived(viewTitles[contextsStore.activeView] ?? 'Задачи');
</script>

<div class="flex h-full flex-col">
	<header class="flex h-14 shrink-0 items-center border-b border-border px-6">
		<h1 class="text-lg font-semibold">{title}</h1>
	</header>

	{#if tasksStore.isStale}
		<div class="shrink-0 border-b border-yellow-500/20 bg-yellow-500/10 px-6 py-2 text-sm text-yellow-600 dark:text-yellow-400">
			Данные могут быть устаревшими — синхронизация с Todoist прервана
		</div>
	{/if}

	{#if contextsStore.activeView === 'weekly'}
		<WeeklyProgress
			weekly_count={tasksStore.meta.weekly_count}
			weekly_limit={tasksStore.meta.weekly_limit}
		/>
	{/if}

	<div class="flex-1 overflow-y-auto p-4">
		{#if tasksStore.loading}
			<p class="py-8 text-center text-sm text-muted-foreground">Загрузка...</p>
		{:else if tasksStore.error}
			<p class="py-8 text-center text-sm text-destructive">{tasksStore.error}</p>
		{:else}
			<TaskList tasks={tasksStore.tasks} />
		{/if}
	</div>
</div>
