<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import TaskList from '$lib/components/TaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import { toggleMode } from 'mode-watcher';
	import { Button } from '$lib/components/ui/button/index.js';

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

	let searchQuery = $state('');

	// Reset search when context/view changes
	$effect(() => {
		contextsStore.activeContextId;
		contextsStore.activeView;
		searchQuery = '';
	});
</script>

<div class="flex h-full flex-col">
	<header class="hidden h-12 shrink-0 items-center border-b border-border/50 px-6 md:flex">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{title}</h1>
		<div class="relative ml-auto mr-2 flex items-center">
			<SearchIcon class="pointer-events-none absolute left-2.5 h-3.5 w-3.5 text-muted-foreground/60" />
			<input
				type="text"
				placeholder="Поиск..."
				bind:value={searchQuery}
				class="h-8 w-48 rounded-md border border-border/50 bg-transparent pl-8 pr-8 text-[13px] text-foreground placeholder:text-muted-foreground/50 focus:border-border focus:outline-none"
			/>
			{#if searchQuery}
				<button
					class="absolute right-2 flex items-center text-muted-foreground/60 hover:text-foreground"
					onclick={() => (searchQuery = '')}
					aria-label="Clear search"
				>
					<XIcon class="h-3.5 w-3.5" />
				</button>
			{/if}
		</div>
		<Button onclick={toggleMode} variant="ghost" size="icon" class="h-8 w-8 text-muted-foreground">
			<SunIcon class="h-4 w-4 scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
			<MoonIcon class="absolute h-4 w-4 scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
			<span class="sr-only">Toggle theme</span>
		</Button>
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
			<TaskList tasks={tasksStore.tasks} {searchQuery} />
		{/if}
	</div>
</div>
