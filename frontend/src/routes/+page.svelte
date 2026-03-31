<script lang="ts">
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { labelFilterStore } from '$lib/stores/label-filter.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { planningStore } from '$lib/stores/planning.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import type { Task } from '$lib/api/types';
	import TaskList from '$lib/components/TaskList.svelte';
	import DayPartTaskList from '$lib/components/DayPartTaskList.svelte';
	import WeeklyProgress from '$lib/components/WeeklyProgress.svelte';
	import BacklogProgress from '$lib/components/BacklogProgress.svelte';
	import PlanningView from '$lib/components/PlanningView.svelte';
	import CreateTaskDialog from '$lib/components/CreateTaskDialog.svelte';
	import NextActionDialog from '$lib/components/NextActionDialog.svelte';

	import { getCompletedTasks } from '$lib/api/client';
	import TaskItem from '$lib/components/TaskItem.svelte';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronsDownUpIcon from '@lucide/svelte/icons/chevrons-down-up';
	import ChevronsDownIcon from '@lucide/svelte/icons/chevrons-down';
	import LinkIcon from '@lucide/svelte/icons/link';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import FilterIcon from '@lucide/svelte/icons/filter';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Toggle } from '$lib/components/ui/toggle/index.js';
	import { t } from 'svelte-intl-precompile';
	import { untrack } from 'svelte';

	let mounted = false;

	$effect(() => {
		// Track context/view changes and refresh tasks
		contextsStore.activeContextId;
		contextsStore.activeView;
		if (mounted && appStore.initialized) tasksStore.refreshWithLoading();
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
	let selectedPriorities = $state<Set<number>>(new Set());
	let selectedLabels = $state<Set<string>>(new Set());
	let filtersExpanded = $state(false);

	const hasActiveFilters = $derived(selectedPriorities.size > 0 || selectedLabels.size > 0);

	// Collect all unique labels from current tasks (excluding day-part labels)
	const dayPartLabels = $derived(new Set(tasksStore.config?.day_parts?.map((dp) => dp.label) ?? []));
	const backlogLabelName = $derived(tasksStore.config?.backlog_label ?? '');
	const weeklyLabelName = $derived(tasksStore.config?.weekly_label ?? '');

	function collectLabels(tasks: Task[]): Set<string> {
		const labels = new Set<string>();
		const view = contextsStore.activeView;
		const hideWeekly = view === 'weekly' || view === 'today' || view === 'tomorrow';
		function walk(t: Task) {
			for (const l of t.labels) {
				if (!dayPartLabels.has(l) && l !== backlogLabelName && !(hideWeekly && l === weeklyLabelName)) labels.add(l);
			}
			for (const c of t.children) walk(c);
		}
		for (const t of tasks) walk(t);
		return labels;
	}

	const allLabels = $derived(Array.from(collectLabels(tasksStore.tasks)).sort());

	function togglePriority(p: number) {
		const next = new Set(selectedPriorities);
		if (next.has(p)) next.delete(p);
		else next.add(p);
		selectedPriorities = next;
	}

	function toggleLabel(label: string) {
		const next = new Set(selectedLabels);
		if (next.has(label)) next.delete(label);
		else next.add(label);
		selectedLabels = next;
		labelFilterStore.clear();
	}

	function clearFilters() {
		selectedPriorities = new Set();
		selectedLabels = new Set();
		labelFilterStore.clear();
	}

	function taskMatchesFilters(task: Task): boolean {
		if (selectedPriorities.size > 0 && !selectedPriorities.has(task.priority)) return false;
		if (selectedLabels.size > 0 && !task.labels.some((l) => selectedLabels.has(l))) return false;
		return true;
	}

	function filterTaskTree(tasks: Task[]): Task[] {
		return tasks.reduce<Task[]>((acc, task) => {
			if (task.is_project_task) {
				const filteredChildren = filterTaskTree(task.children);
				if (filteredChildren.length > 0) {
					acc.push({ ...task, children: filteredChildren });
				}
				return acc;
			}
			if (taskMatchesFilters(task)) {
				acc.push(task);
			} else {
				const filteredChildren = filterTaskTree(task.children);
				if (filteredChildren.length > 0) {
					acc.push({ ...task, children: filteredChildren });
				}
			}
			return acc;
		}, []);
	}

	const URL_RE = /https?:\/\/\S+/;

	function taskHasLink(task: Task): boolean {
		if (URL_RE.test(task.content) || URL_RE.test(task.description)) return true;
		return task.children.some(taskHasLink);
	}

	const filteredTasks = $derived.by(() => {
		let tasks = tasksStore.tasks;
		if (linksOnly) tasks = tasks.filter(taskHasLink);
		if (hasActiveFilters) tasks = filterTaskTree(tasks);
		return tasks;
	});

	const activeContextName = $derived.by(() => {
		const id = contextsStore.activeContextId;
		if (!id) return '';
		return contextsStore.contexts.find((c) => c.id === id)?.display_name ?? '';
	});

	function resetContext() {
		contextsStore.setContext(null);
	}

	// Reset search and filters when context/view changes; restore persisted filters for 'all' view
	$effect(() => {
		contextsStore.activeContextId;
		const view = contextsStore.activeView;
		const label = labelFilterStore.activeLabel;
		searchQuery = '';
		if (view === 'all' && !label) {
			const saved = untrack(() => appStore.allFilters);
			if (saved) {
				linksOnly = saved.links_only;
				selectedPriorities = new Set(saved.selected_priorities);
				selectedLabels = new Set(saved.selected_labels);
			} else {
				linksOnly = false;
				selectedPriorities = new Set();
				selectedLabels = new Set();
			}
			filtersExpanded = true; // Always expanded on All tasks view
		} else {
			linksOnly = false;
			selectedPriorities = new Set();
			selectedLabels = label ? new Set([label]) : new Set();
			filtersExpanded = !!label;
		}
	});

	// Persist filters when they change while in 'all' view
	$effect(() => {
		if (contextsStore.activeView !== 'all') return;
		selectedPriorities;
		selectedLabels;
		linksOnly;
		filtersExpanded;
		appStore.saveAllFilters({
			selected_priorities: Array.from(selectedPriorities),
			selected_labels: Array.from(selectedLabels),
			links_only: linksOnly,
			filters_expanded: filtersExpanded
		});
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
	const quickCaptureOpen = $derived(appStore.quickCaptureOpen);
	let createDayPartLabel = $state('');

	// Completed-today tasks for the Today view
	let completedTodayTasks = $state<import('$lib/api/types').Task[]>([]);
	let completedTodayLoading = $state(false);
	let completedTodayExpanded = $state(false);

	$effect(() => {
		if (contextsStore.activeView !== 'today') {
			completedTodayTasks = [];
			return;
		}
		completedTodayLoading = true;
		const todayDate = todayStr();
		getCompletedTasks(contextsStore.activeContextId ?? undefined)
			.then((res) => {
				completedTodayTasks = res.tasks.filter(
					(t) => t.completed_at && t.completed_at.slice(0, 10) === todayDate
				);
			})
			.catch(() => { completedTodayTasks = []; })
			.finally(() => { completedTodayLoading = false; });
	});

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
			createDayPartLabel = '';
			createDialogOpen = true;
		} else if (e.key === 'i') {
			e.preventDefault();
			appStore.quickCaptureOpen = true;
		}
	}}
/>

{#if planningStore.active}
	<PlanningView />
{:else}

<div class="flex h-full flex-col">
	<!-- Desktop header -->
	<header class="hidden h-12 shrink-0 items-center border-b border-border/50 px-6 md:flex">
		<h1 class="shrink-0 text-sm font-semibold tracking-wide text-foreground">{title}</h1>
		{#if contextsStore.contexts.length > 0}
			<div class="ml-4 flex items-center gap-0.5">
				{#each contextsStore.contexts as ctx (ctx.id)}
					<button
						class="flex h-6 items-center gap-1.5 rounded-md px-2 text-[11px] font-medium transition-colors
							{contextsStore.activeContextId === ctx.id ? 'bg-accent text-foreground' : 'text-muted-foreground/60 hover:bg-accent/50 hover:text-foreground'}"
						onclick={() => contextsStore.setContext(ctx.id)}
					>
						{#if ctx.color}
							<span class="h-1.5 w-1.5 shrink-0 rounded-full" style="background-color: {ctx.color}; opacity: {contextsStore.activeContextId === ctx.id ? 1 : 0.5}"></span>
						{/if}
						{ctx.display_name}
					</button>
				{/each}
				<button
					class="flex h-6 items-center rounded-md px-2 text-[11px] font-medium transition-colors
						{contextsStore.activeContextId === null ? 'bg-accent text-foreground' : 'text-muted-foreground/60 hover:bg-accent/50 hover:text-foreground'}"
					onclick={() => contextsStore.setContext(null)}
				>
					{$t('sidebar.all')}
				</button>
			</div>
		{/if}
		{#if !isCompletedView}
			<Button onclick={() => { createDayPartLabel = ''; createDialogOpen = true; }} variant="ghost" size="icon" class="ml-auto me-1 h-8 w-8 text-muted-foreground hover:text-foreground" title="Add task (Q)" disabled={isBacklogAtLimit}>
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
		{#if !isCompletedView && contextsStore.activeView !== 'all'}
			<Toggle bind:pressed={filtersExpanded} size="sm" class="me-1 h-7 w-7 text-muted-foreground {hasActiveFilters ? 'text-primary' : ''}" title="Filters">
				<FilterIcon class="h-2.5 w-2.5" />
				<span class="sr-only">Filters</span>
			</Toggle>
		{/if}
		<Button onclick={handleSync} variant="ghost" size="icon" class="h-8 w-8 text-muted-foreground" title="Sync" disabled={syncing}>
			<RefreshCwIcon class="h-4 w-4 {syncing ? 'animate-spin' : ''}" />
			<span class="sr-only">Sync</span>
		</Button>
	</header>

	<!-- Mobile header -->
	<header class="flex shrink-0 items-center gap-2 border-b border-border/50 px-3 py-2 md:hidden">
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
		{#if !isCompletedView && contextsStore.activeView !== 'all'}
			<Toggle bind:pressed={filtersExpanded} size="sm" class="h-8 w-8 shrink-0 text-muted-foreground {hasActiveFilters ? 'text-primary' : ''}" title="Filters">
				<FilterIcon class="h-3 w-3" />
				<span class="sr-only">Filters</span>
			</Toggle>
		{/if}
		<Button onclick={handleSync} variant="ghost" size="icon" class="h-8 w-8 shrink-0 text-muted-foreground" title="Sync" disabled={syncing}>
			<RefreshCwIcon class="h-4 w-4 {syncing ? 'animate-spin' : ''}" />
			<span class="sr-only">Sync</span>
		</Button>
	</header>

	<!-- Mobile context strip -->
	{#if contextsStore.contexts.length > 0}
		<div class="flex shrink-0 gap-0.5 overflow-x-auto border-b border-border/50 px-3 py-1.5 md:hidden">
			{#each contextsStore.contexts as ctx (ctx.id)}
				<button
					class="flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors
						{contextsStore.activeContextId === ctx.id ? 'bg-accent text-foreground' : 'text-muted-foreground/60 hover:bg-accent/50 hover:text-foreground'}"
					onclick={() => contextsStore.setContext(ctx.id)}
				>
					{#if ctx.color}
						<span class="h-1.5 w-1.5 shrink-0 rounded-full" style="background-color: {ctx.color}; opacity: {contextsStore.activeContextId === ctx.id ? 1 : 0.5}"></span>
					{/if}
					{ctx.display_name}
				</button>
			{/each}
			<button
				class="flex shrink-0 items-center rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors
					{contextsStore.activeContextId === null ? 'bg-accent text-foreground' : 'text-muted-foreground/60 hover:bg-accent/50 hover:text-foreground'}"
				onclick={() => contextsStore.setContext(null)}
			>
				{$t('sidebar.all')}
			</button>
		</div>
	{/if}

	<!-- Filter bar -->
	{#if filtersExpanded && !isCompletedView}
		<div class="shrink-0 border-b border-border/50 px-3 py-2 md:px-6">
			<div class="flex flex-wrap items-start gap-3">
				<!-- Priority filter -->
				<div class="flex items-center gap-1.5">
					<span class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground/60">{$t('filters.priority')}</span>
					{#each [
						{ value: 4, label: 'P1', color: 'text-red-500', bg: 'bg-red-500/15 border-red-500/30', border: 'border-red-500/20' },
						{ value: 3, label: 'P2', color: 'text-amber-500', bg: 'bg-amber-500/15 border-amber-500/30', border: 'border-amber-500/20' },
						{ value: 2, label: 'P3', color: 'text-blue-400', bg: 'bg-blue-400/15 border-blue-400/30', border: 'border-blue-400/20' },
						{ value: 1, label: 'P4', color: 'text-muted-foreground', bg: 'bg-muted border-muted-foreground/30', border: 'border-border' }
					] as p (p.value)}
						<button
							class="flex h-6 items-center gap-1 rounded-md border px-1.5 text-[11px] font-medium transition-colors
								{selectedPriorities.has(p.value) ? p.bg + ' ' + p.color : 'border-transparent ' + p.color + ' hover:' + p.border + ' opacity-50 hover:opacity-100'}"
							onclick={() => togglePriority(p.value)}
						>
							<FlagIcon class="h-3 w-3" />
							{p.label}
						</button>
					{/each}
				</div>

				<!-- Labels filter -->
				{#if allLabels.length > 0}
					<div class="flex flex-wrap items-center gap-1.5">
						<span class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground/60">{$t('filters.labels')}</span>
						{#each allLabels as label (label)}
							<button
								class="flex h-6 items-center rounded-md border px-2 text-[11px] font-medium transition-colors
									{selectedLabels.has(label)
										? 'border-primary/30 bg-primary/15 text-primary'
										: 'border-transparent text-muted-foreground opacity-50 hover:border-border hover:opacity-100'}"
								onclick={() => toggleLabel(label)}
							>
								{label}
							</button>
						{/each}
					</div>
				{/if}

				<!-- Clear filters -->
				{#if hasActiveFilters}
					<button
						class="flex h-6 items-center gap-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
						onclick={clearFilters}
					>
						<XIcon class="h-3 w-3" />
						{$t('filters.clearAll')}
					</button>
				{/if}
			</div>
		</div>
	{/if}

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

	<div class="flex-1 overflow-y-auto px-1 pb-20 pt-2 md:px-3 md:py-3">
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
					oncreate={(label) => { createDayPartLabel = label; createDialogOpen = true; }}
				/>
			{:else}
				<TaskList tasks={filteredTasks} {searchQuery} completed={isCompletedView} contextName={activeContextName} onResetContext={resetContext} />
			{/if}
		{/if}
		<div class="h-4"></div>

		<!-- Completed today section -->
		{#if contextsStore.activeView === 'today' && !tasksStore.loading}
			{#if completedTodayLoading}
				<div class="flex justify-center py-4">
					<div class="h-4 w-4 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-transparent"></div>
				</div>
			{:else if completedTodayTasks.length > 0}
				<div class="mt-2 px-1 pb-4 md:px-3">
					<button
						class="mb-2 flex w-full items-center gap-2 px-2 md:px-3"
						onclick={() => { completedTodayExpanded = !completedTodayExpanded; }}
					>
						<div class="h-px flex-1 bg-border/40"></div>
						<span class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/40 hover:text-muted-foreground/70 transition-colors">
							{$t('tasks.completedToday')}
							<span class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium normal-case tracking-normal text-muted-foreground/60">{completedTodayTasks.length}</span>
							<svg class="h-3 w-3 transition-transform {completedTodayExpanded ? 'rotate-180' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m6 9 6 6 6-6"/></svg>
						</span>
						<div class="h-px flex-1 bg-border/40"></div>
					</button>
					{#if completedTodayExpanded}
						<div class="opacity-60">
							{#each completedTodayTasks as task (task.id)}
								<TaskItem {task} completed={true} />
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>

{#if !isCompletedView && !isBacklogAtLimit}
	<!-- Mobile FAB -->
	<button
		class="fixed bottom-6 right-6 z-40 flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg transition-transform hover:scale-105 active:scale-95 md:hidden"
		onclick={() => { createDayPartLabel = ''; createDialogOpen = true; }}
		aria-label="Add task"
	>
		<PlusIcon class="h-5 w-5" />
	</button>
{/if}

<CreateTaskDialog bind:open={createDialogOpen} dueDate={createDueDate} dayPartLabel={createDayPartLabel} />
{#if !isCompletedView}
	<NextActionDialog />
{/if}

{/if}
