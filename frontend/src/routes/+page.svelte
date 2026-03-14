<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import type { Task } from '$lib/api/types';
	import TaskList from '$lib/components/TaskList.svelte';
	import DayPartTaskList from '$lib/components/DayPartTaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';
	import CreateTaskDialog from '$lib/components/CreateTaskDialog.svelte';
	import NextActionDialog from '$lib/components/NextActionDialog.svelte';
	import QuickCaptureButton from '$lib/components/QuickCaptureButton.svelte';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronsDownUpIcon from '@lucide/svelte/icons/chevrons-down-up';
	import ChevronsDownIcon from '@lucide/svelte/icons/chevrons-down';
	import LinkIcon from '@lucide/svelte/icons/link';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import { toggleMode } from 'mode-watcher';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Toggle } from '$lib/components/ui/toggle/index.js';

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
		inbox: 'Входящие',
		today: 'Сегодня',
		tomorrow: 'Завтра',
		weekly: 'На неделе',
		'next-week': 'На следующей неделе',
		completed: 'Выполненные'
	};

	const title = $derived(viewTitles[contextsStore.activeView] ?? 'Задачи');
	const isCompletedView = $derived(contextsStore.activeView === 'completed');

	let searchQuery = $state('');
	let linksOnly = $state(false);

	const URL_RE = /https?:\/\/\S+/;

	function taskHasLink(task: Task): boolean {
		if (URL_RE.test(task.content) || URL_RE.test(task.description)) return true;
		return task.children.some(taskHasLink);
	}

	const filteredTasks = $derived(
		linksOnly ? tasksStore.tasks.filter(taskHasLink) : tasksStore.tasks
	);

	const activeContextName = $derived.by(() => {
		const id = contextsStore.activeContextId;
		if (!id) return '';
		return contextsStore.contexts.find((c) => c.id === id)?.display_name ?? '';
	});

	function resetContext() {
		contextsStore.setContext(null);
	}

	// Reset search and link filter when context/view changes
	$effect(() => {
		contextsStore.activeContextId;
		contextsStore.activeView;
		searchQuery = '';
		linksOnly = false;
	});

	function collectParentIds(tasks: Task[]): string[] {
		const result: string[] = [];
		function walk(t: Task) {
			if (t.sub_task_count > 0) result.push(t.id);
			for (const c of t.children) walk(c);
		}
		for (const t of tasks) walk(t);
		return result;
	}

	function toggleAllSubtasks() {
		if (collapsedStore.hasAny) {
			collapsedStore.expandAll();
		} else {
			collapsedStore.collapseAll(collectParentIds(tasksStore.tasks));
		}
	}

	let createDialogOpen = $state(false);
	let quickCaptureOpen = $state(false);
</script>

<svelte:window
	onkeydown={(e) => {
		const tag = (e.target as HTMLElement)?.tagName;
		if (tag === 'INPUT' || tag === 'TEXTAREA') return;
		if (e.ctrlKey || e.metaKey || e.altKey) return;
		if (createDialogOpen || quickCaptureOpen) return;

		if (e.key === 'q') {
			e.preventDefault();
			createDialogOpen = true;
		} else if (e.key === 'i') {
			e.preventDefault();
			quickCaptureOpen = true;
		}
	}}
/>

<div class="flex h-full flex-col">
	<header class="hidden h-12 shrink-0 items-center border-b border-border/50 px-6 md:flex">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{title}</h1>
		{#if !isCompletedView}
			<Button onclick={() => (createDialogOpen = true)} variant="ghost" size="icon" class="ml-auto me-1 h-8 w-8 text-muted-foreground hover:text-foreground" title="Add task (Q)">
				<PlusIcon class="h-4 w-4" />
				<span class="sr-only">Add task</span>
			</Button>
			<Toggle bind:pressed={linksOnly} size="sm" class="me-1 h-7 w-7 text-muted-foreground" title="Show only tasks with links">
				<LinkIcon class="h-3.5 w-3.5" />
				<span class="sr-only">Filter by links</span>
			</Toggle>
			<Button onclick={toggleAllSubtasks} variant="ghost" size="icon" class="me-2 h-8 w-8 text-muted-foreground">
				{#if collapsedStore.hasAny}
					<ChevronsDownIcon class="h-4 w-4" />
				{:else}
					<ChevronsDownUpIcon class="h-4 w-4" />
				{/if}
				<span class="sr-only">Toggle all subtasks</span>
			</Button>
			<div class="relative mr-2 flex items-center">
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
		{:else}
			<div class="ml-auto"></div>
		{/if}
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
			{#if (contextsStore.activeView === 'today' || contextsStore.activeView === 'tomorrow') && (tasksStore.config?.day_parts?.length ?? 0) > 0}
				<DayPartTaskList
					tasks={filteredTasks}
					dayParts={tasksStore.config!.day_parts}
					timezone={tasksStore.config!.timezone}
					view={contextsStore.activeView === 'tomorrow' ? 'tomorrow' : 'today'}
					{searchQuery}
					contextName={activeContextName}
					onResetContext={resetContext}
				/>
			{:else}
				<TaskList tasks={filteredTasks} {searchQuery} completed={isCompletedView} contextName={activeContextName} onResetContext={resetContext} />
			{/if}
		{/if}
	</div>
</div>

{#if !isCompletedView}
	<!-- Mobile FAB -->
	<button
		class="fixed bottom-6 right-6 z-40 flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 active:scale-95 md:hidden"
		onclick={() => (createDialogOpen = true)}
		aria-label="Add task"
	>
		<PlusIcon class="h-5 w-5" />
	</button>
{/if}

<CreateTaskDialog bind:open={createDialogOpen} />
{#if !isCompletedView}
	<NextActionDialog />
{/if}
<QuickCaptureButton bind:open={quickCaptureOpen} />
