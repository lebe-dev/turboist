<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { planningStore } from '$lib/stores/planning.svelte';
	import type { Task } from '$lib/api/types';
	import TaskList from '$lib/components/TaskList.svelte';
	import DayPartTaskList from '$lib/components/DayPartTaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';
	import BacklogProgress from '$lib/components/BacklogProgress.svelte';
	import PlanningView from '$lib/components/PlanningView.svelte';
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
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Toggle } from '$lib/components/ui/toggle/index.js';
	import { t } from 'svelte-intl-precompile';

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
		if (mounted) tasksStore.refreshWithLoading();
		mounted = true;
	});

	const viewTitleKeys: Record<string, string> = {
		all: 'views.all',
		inbox: 'views.inbox',
		today: 'views.today',
		tomorrow: 'views.tomorrow',
		weekly: 'views.weekly',
		backlog: 'views.backlog',
		completed: 'views.completed'
	};

	const title = $derived($t(viewTitleKeys[contextsStore.activeView] ?? 'views.tasks'));
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

	let syncing = $state(false);

	async function handleSync() {
		if (syncing) return;
		syncing = true;
		try {
			await tasksStore.refresh();
		} finally {
			syncing = false;
		}
	}

	let createDialogOpen = $state(false);
	let quickCaptureOpen = $state(false);

	function todayStr(): string {
		const d = new Date();
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	function tomorrowStr(): string {
		const d = new Date();
		d.setDate(d.getDate() + 1);
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	const createDueDate = $derived.by(() => {
		if (contextsStore.activeView === 'today') return todayStr();
		if (contextsStore.activeView === 'tomorrow') return tomorrowStr();
		return '';
	});

	const isBacklogAtLimit = $derived(
		contextsStore.activeView === 'backlog' &&
		tasksStore.meta.backlog_limit > 0 &&
		tasksStore.meta.backlog_count >= tasksStore.meta.backlog_limit
	);
</script>

<svelte:window
	onkeydown={(e) => {
		if (planningStore.active) return;
		const tag = (e.target as HTMLElement)?.tagName;
		if (tag === 'INPUT' || tag === 'TEXTAREA') return;
		if (e.ctrlKey || e.metaKey || e.altKey) return;
		if (createDialogOpen || quickCaptureOpen) return;

		if (e.key === 'q' && !isBacklogAtLimit) {
			e.preventDefault();
			createDialogOpen = true;
		} else if (e.key === 'i') {
			e.preventDefault();
			quickCaptureOpen = true;
		}
	}}
/>

{#if planningStore.active}
	<PlanningView />
{:else}

<div class="flex h-full flex-col">
	<!-- Desktop header -->
	<header class="hidden h-12 shrink-0 items-center border-b border-border/50 px-6 md:flex">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{title}</h1>
		{#if !isCompletedView}
			<Button onclick={() => (createDialogOpen = true)} variant="ghost" size="icon" class="ml-auto me-1 h-8 w-8 text-muted-foreground hover:text-foreground" title="Add task (Q)" disabled={isBacklogAtLimit}>
				<PlusIcon class="h-4 w-4" />
				<span class="sr-only">Add task</span>
			</Button>
			<Toggle bind:pressed={linksOnly} size="sm" class="me-1 h-7 w-7 text-muted-foreground" title="Show only tasks with links">
				<LinkIcon class="h-2.5 w-2.5" />
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
					placeholder={$t('tasks.search')}
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
		<Button onclick={handleSync} variant="ghost" size="icon" class="h-8 w-8 text-muted-foreground" title="Sync" disabled={syncing}>
			<RefreshCwIcon class="h-4 w-4 {syncing ? 'animate-spin' : ''}" />
			<span class="sr-only">Sync</span>
		</Button>
	</header>

	<!-- Mobile header -->
	<header class="flex shrink-0 items-center gap-2 border-b border-border/50 px-3 py-2 md:hidden">
		<h1 class="shrink-0 text-sm font-semibold tracking-wide text-foreground">{title}</h1>
		{#if !isCompletedView}
			<div class="relative flex min-w-0 flex-1 items-center">
				<SearchIcon class="pointer-events-none absolute left-2.5 h-3.5 w-3.5 text-muted-foreground/60" />
				<input
					type="text"
					placeholder={$t('tasks.search')}
					bind:value={searchQuery}
					class="h-8 w-full rounded-md border border-border/50 bg-transparent pl-8 pr-8 text-[13px] text-foreground placeholder:text-muted-foreground/50 focus:border-border focus:outline-none"
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
			<Toggle bind:pressed={linksOnly} size="sm" class="h-8 w-8 shrink-0 text-muted-foreground" title="Show only tasks with links">
				<LinkIcon class="h-3 w-3" />
				<span class="sr-only">Filter by links</span>
			</Toggle>
			<Button onclick={toggleAllSubtasks} variant="ghost" size="icon" class="h-8 w-8 shrink-0 text-muted-foreground">
				{#if collapsedStore.hasAny}
					<ChevronsDownIcon class="h-4 w-4" />
				{:else}
					<ChevronsDownUpIcon class="h-4 w-4" />
				{/if}
				<span class="sr-only">Toggle all subtasks</span>
			</Button>
		{/if}
		<Button onclick={handleSync} variant="ghost" size="icon" class="h-8 w-8 shrink-0 text-muted-foreground" title="Sync" disabled={syncing}>
			<RefreshCwIcon class="h-4 w-4 {syncing ? 'animate-spin' : ''}" />
			<span class="sr-only">Sync</span>
		</Button>
	</header>

	{#if tasksStore.isStale}
		<div class="flex shrink-0 items-center gap-2 border-b border-yellow-500/10 bg-yellow-500/5 px-3 py-2 md:px-6">
			<TriangleAlertIcon class="h-3.5 w-3.5 text-yellow-500/70" />
			<span class="text-[12px] text-yellow-500/70">{$t('tasks.staleWarning')}</span>
		</div>
	{/if}

	{#if contextsStore.activeView === 'weekly'}
		<WeeklyProgress
			weekly_count={tasksStore.meta.weekly_count}
			weekly_limit={tasksStore.meta.weekly_limit}
		/>
	{/if}

	{#if contextsStore.activeView === 'backlog'}
		<BacklogProgress
			backlog_count={tasksStore.meta.backlog_count}
			backlog_limit={tasksStore.meta.backlog_limit}
		/>
	{/if}

	<div class="flex-1 overflow-y-auto px-1 py-2 md:px-3 md:py-3">
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

{#if !isCompletedView && !isBacklogAtLimit}
	<!-- Mobile FAB -->
	<button
		class="fixed bottom-6 right-6 z-40 flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 active:scale-95 md:hidden"
		onclick={() => (createDialogOpen = true)}
		aria-label="Add task"
	>
		<PlusIcon class="h-5 w-5" />
	</button>
{/if}

<CreateTaskDialog bind:open={createDialogOpen} dueDate={createDueDate} />
{#if !isCompletedView}
	<NextActionDialog />
{/if}
<QuickCaptureButton bind:open={quickCaptureOpen} />

{/if}
