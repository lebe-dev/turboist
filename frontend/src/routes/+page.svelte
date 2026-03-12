<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import TaskList from '$lib/components/TaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

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
	<header class="hidden h-12 shrink-0 items-center border-b border-border/50 px-6 md:flex">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{title}</h1>
	</header>

	{#if tasksStore.isStale}
		<div class="flex shrink-0 items-center gap-2 border-b border-yellow-500/10 bg-yellow-500/5 px-6 py-2">
			<TriangleAlertIcon class="h-3.5 w-3.5 text-yellow-500/70" />
			<span class="text-[12px] text-yellow-500/70">Данные могут быть устаревшими</span>
		</div>
	{/if}

	{#if contextsStore.activeView === 'weekly'}
		<WeeklyProgress
			weekly_count={tasksStore.meta.weekly_count}
			weekly_limit={tasksStore.meta.weekly_limit}
		/>
	{/if}

	<div class="flex-1 overflow-y-auto px-3 py-3">
		{#if tasksStore.loading}
			<div class="flex items-center justify-center py-20">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
			</div>
		{:else if tasksStore.error}
			<p class="py-8 text-center text-sm text-destructive/80">{tasksStore.error}</p>
		{:else}
			<TaskList tasks={tasksStore.tasks} />
		{/if}
	</div>
</div>
