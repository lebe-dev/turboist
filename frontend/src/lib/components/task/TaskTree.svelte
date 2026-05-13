<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { buildTree, type TaskNode } from '$lib/utils/taskTree';
	import type { ListMutator } from '$lib/utils/taskActions';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import {
		buildProjectsById,
		buildTasksById,
		isTaskVisible
	} from '$lib/utils/visibility';
	import { getContext } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import { t } from '$lib/i18n';
	import type { SubtaskCollapseCtx } from '$lib/context/subtaskCollapse';
	import { SUBTASK_COLLAPSE_KEY } from '$lib/context/subtaskCollapse';
	import TaskItem from './TaskItem.svelte';
	import Self from './TaskTree.svelte';

	let {
		tasks,
		nodes,
		depth = 0,
		showProject = true,
		hideTodayBadge = false,
		hideTomorrowBadge = false,
		hideDue = false,
		draggable = false,
		showUnplannedBadge = false,
		collapseCompletedChildren = false,
		collapsibleSubtasks = false,
		collapsedIds,
		mutator,
		belongs,
		onToggle,
		visibleIds
	}: {
		tasks?: Task[];
		nodes?: TaskNode[];
		depth?: number;
		showProject?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
		hideDue?: boolean;
		draggable?: boolean;
		showUnplannedBadge?: boolean;
		collapseCompletedChildren?: boolean;
		collapsibleSubtasks?: boolean;
		collapsedIds?: Set<number>;
		mutator?: ListMutator;
		belongs?: (task: Task) => boolean;
		onToggle?: (task: Task) => void;
		visibleIds?: number[];
	} = $props();

	const collapseCtx = getContext<SubtaskCollapseCtx | undefined>(SUBTASK_COLLAPSE_KEY);

	// Root-level TaskTree owns the collapsed state when no context is provided;
	// nested Self receives it via prop (collapsedIds) to stay in sync.
	let ownCollapsedIds = $state(new Set<number>());
	const effectiveCollapsedIds = $derived(
		collapseCtx ? collapseCtx.ids : (collapsedIds ?? ownCollapsedIds)
	);

	function toggleCollapse(id: number): void {
		if (collapseCtx) {
			collapseCtx.toggle(id);
			return;
		}
		const src = collapsedIds ?? ownCollapsedIds;
		const next = new SvelteSet(src);
		if (next.has(id)) next.delete(id); else next.add(id);
		ownCollapsedIds = next;
	}

	let completedChildrenOpen: Record<number, boolean> = $state({});

	const visibleTasks = $derived.by(() => {
		if (!tasks) return undefined;
		if (!settingsStore.publicView) return tasks;
		const tasksById = buildTasksById(tasks);
		const projectsById = buildProjectsById(projectsStore.items ?? []);
		return tasks.filter((t) => isTaskVisible(t, true, projectsById, tasksById));
	});
	const resolved = $derived<TaskNode[]>(nodes ?? (visibleTasks ? buildTree(visibleTasks) : []));

	function flattenIds(ns: TaskNode[], out: number[]): void {
		for (const n of ns) {
			out.push(n.task.id);
			if (n.children.length > 0) flattenIds(n.children, out);
		}
	}
	const effectiveVisibleIds = $derived.by<number[]>(() => {
		if (visibleIds) return visibleIds;
		const flat: number[] = [];
		flattenIds(resolved, flat);
		return flat;
	});
</script>

<div class={depth === 0 ? 'flex flex-col' : 'flex flex-col divide-y divide-border/40'}>
	{#each resolved as node (node.task.id)}
		{@const openChildren = collapseCompletedChildren ? node.children.filter((c) => c.task.status !== 'completed') : node.children}
		{@const doneChildren = collapseCompletedChildren ? node.children.filter((c) => c.task.status === 'completed') : []}
		{@const doneOpen = completedChildrenOpen[node.task.id] ?? false}
		{@const subtasksCollapsed = effectiveCollapsedIds.has(node.task.id)}
		<div class={depth === 0 ? 'border-b border-border/60 last:border-b-0 pb-1 pt-0.5' : 'contents'}>
			<TaskItem
				task={node.task}
				{depth}
				{showProject}
				{hideTodayBadge}
				{hideTomorrowBadge}
				{hideDue}
				{draggable}
				{showUnplannedBadge}
				{mutator}
				{belongs}
				{onToggle}
				hasSubtasks={node.children.length > 0}
				{subtasksCollapsed}
				onToggleCollapse={collapsibleSubtasks && node.children.length > 0 ? () => toggleCollapse(node.task.id) : undefined}
				visibleIds={effectiveVisibleIds}
			/>
			{#if openChildren.length > 0 && !subtasksCollapsed}
				<Self
					nodes={openChildren}
					depth={depth + 1}
					{showProject}
					{hideTodayBadge}
					{hideTomorrowBadge}
					{hideDue}
					{draggable}
					{showUnplannedBadge}
					{collapseCompletedChildren}
					{collapsibleSubtasks}
					collapsedIds={effectiveCollapsedIds}
					{mutator}
					{belongs}
					{onToggle}
					visibleIds={effectiveVisibleIds}
				/>
			{/if}
			{#if doneChildren.length > 0 && !subtasksCollapsed}
				<button
					type="button"
					class="flex items-center gap-1.5 px-3 py-1.5 text-left text-xs text-muted-foreground hover:text-foreground"
					style:padding-left={`${(depth + 1) * 1.5 + 0.75}rem`}
					onclick={() => { completedChildrenOpen[node.task.id] = !doneOpen; }}
					aria-expanded={doneOpen}
				>
					{#if doneOpen}
						<CaretDownIcon class="size-3 shrink-0" />
					{:else}
						<CaretRightIcon class="size-3 shrink-0" />
					{/if}
					<CheckCircleIcon class="size-3.5 shrink-0" weight="fill" />
					<span>{$t('nav.completed')}</span>
					<span class="text-muted-foreground/70">{doneChildren.length}</span>
				</button>
				{#if doneOpen}
					<Self
						nodes={doneChildren}
						depth={depth + 1}
						{showProject}
						{hideTodayBadge}
						{hideTomorrowBadge}
						{hideDue}
						{draggable}
						{showUnplannedBadge}
						{collapseCompletedChildren}
						{mutator}
						{belongs}
						{onToggle}
						visibleIds={effectiveVisibleIds}
					/>
				{/if}
			{/if}
		</div>
	{/each}
</div>
