<script lang="ts">
	import { page } from '$app/stores';
	import { appStore } from '$lib/stores/app.svelte';
	import { projectTasksStore } from '$lib/stores/project-tasks.svelte';
	import { collapsedStore } from '$lib/stores/collapsed.svelte';
	import { sectionsStore } from '$lib/stores/sections.svelte';
	import type { Task } from '$lib/api/types';
	import TaskItem from '$lib/components/TaskItem.svelte';
	import CreateTaskDialog from '$lib/components/CreateTaskDialog.svelte';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ChevronsDownUpIcon from '@lucide/svelte/icons/chevrons-down-up';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import PinIcon from '@lucide/svelte/icons/pin';
	import PinOffIcon from '@lucide/svelte/icons/pin-off';
	import { t } from 'svelte-intl-precompile';

	const projectId = $derived($page.params.id ?? '');

	const project = $derived(appStore.projects.find((p) => p.id === projectId));

	const tasks = $derived(projectTasksStore.getProjectTasks(projectId));

	interface SectionGroup {
		id: string | null;
		name: string;
		tasks: Task[];
	}

	const sectionGroups = $derived.by(() => {
		if (!project) return [];

		const groups: SectionGroup[] = [];
		const tasksBySectionId = new Map<string | null, Task[]>();

		for (const task of tasks) {
			const key = task.section_id;
			const list = tasksBySectionId.get(key) ?? [];
			list.push(task);
			tasksBySectionId.set(key, list);
		}

		// Named sections first, in project order
		for (const section of project.sections) {
			const sectionTasks = tasksBySectionId.get(section.id);
			if (sectionTasks && sectionTasks.length > 0) {
				groups.push({ id: section.id, name: section.name, tasks: sectionTasks });
			}
		}

		// Tasks without section
		const noSectionTasks = tasksBySectionId.get(null);
		if (noSectionTasks && noSectionTasks.length > 0) {
			groups.push({ id: null, name: $t('projects.noSection'), tasks: noSectionTasks });
		}

		// Pinned sections first (stable partition)
		const pinned: SectionGroup[] = [];
		const rest: SectionGroup[] = [];
		for (const g of groups) {
			if (g.id && sectionsStore.isPinned(g.id)) pinned.push(g);
			else rest.push(g);
		}
		return [...pinned, ...rest];
	});

	// Collect IDs of all tasks that have subtasks in the current project view
	function collectParentIds(list: Task[], acc: string[]): void {
		for (const task of list) {
			if (task.children && task.children.length > 0) {
				acc.push(task.id);
				collectParentIds(task.children, acc);
			}
		}
	}

	const parentTaskIds = $derived.by(() => {
		const acc: string[] = [];
		for (const group of sectionGroups) {
			collectParentIds(group.tasks, acc);
		}
		return acc;
	});

	const allSubtasksCollapsed = $derived(collapsedStore.allCollapsed(parentTaskIds));

	function toggleAllSubtasks() {
		if (allSubtasksCollapsed) {
			collapsedStore.expandMany(parentTaskIds);
		} else {
			collapsedStore.collapseMany(parentTaskIds);
		}
	}

	let createDialogOpen = $state(false);
	let createSectionId = $state('');

	function openCreateDialog(sectionId: string | null) {
		createSectionId = sectionId ?? '';
		createDialogOpen = true;
	}

	function handleTaskCreated() {
		projectTasksStore.refresh();
	}
</script>

<div class="flex h-full flex-col">
	<header class="flex h-12 shrink-0 items-center gap-2.5 border-b border-border/50 px-6">
		<FolderIcon class="h-4 w-4 shrink-0 text-muted-foreground" />
		<h1 class="text-sm font-semibold tracking-wide text-foreground">
			{project?.name ?? 'Project'}
		</h1>
		{#if parentTaskIds.length > 0}
			<button
				class="ml-auto flex h-7 w-7 items-center justify-center rounded text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
				onclick={toggleAllSubtasks}
				title={allSubtasksCollapsed ? $t('projects.expandAllSubtasks') : $t('projects.collapseAllSubtasks')}
				aria-label={allSubtasksCollapsed ? $t('projects.expandAllSubtasks') : $t('projects.collapseAllSubtasks')}
			>
				{#if allSubtasksCollapsed}
					<ChevronsUpDownIcon class="h-3.5 w-3.5" />
				{:else}
					<ChevronsDownUpIcon class="h-3.5 w-3.5" />
				{/if}
			</button>
		{/if}
	</header>

	<div class="flex-1 overflow-y-auto px-1 pb-20 pt-2 md:px-3 md:py-3">
		{#if projectTasksStore.loading && !projectTasksStore.loaded}
			<div class="flex items-center justify-center py-20">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
			</div>
		{:else if tasks.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
				<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
				<p class="text-sm">{$t('projects.noTasks')}</p>
			</div>
		{:else}
			{#each sectionGroups as group (group.id ?? '__none')}
				{@const sectionCollapsed = group.id ? sectionsStore.isCollapsed(group.id) : false}
				{@const sectionPinned = group.id ? sectionsStore.isPinned(group.id) : false}
				<div class="mb-4">
					<div class="group/sec mb-2 flex items-center gap-2 px-2 md:px-3">
						{#if group.id}
							<button
								class="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/50 transition-colors hover:bg-accent hover:text-foreground"
								onclick={() => sectionsStore.toggleCollapsed(group.id!)}
								title={sectionCollapsed ? $t('projects.expandSection') : $t('projects.collapseSection')}
								aria-label={sectionCollapsed ? $t('projects.expandSection') : $t('projects.collapseSection')}
							>
								<ChevronRightIcon class="h-3.5 w-3.5 transition-transform duration-150 {sectionCollapsed ? '' : 'rotate-90'}" />
							</button>
						{/if}
						<div class="h-px flex-1 bg-border/40"></div>
						<span class="text-[11px] font-semibold uppercase tracking-wider {sectionPinned ? 'text-primary/70' : 'text-muted-foreground/50'}">
							{group.name}
						</span>
						<span class="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground/60">
							{group.tasks.length}
						</span>
						{#if group.id}
							<button
								class="flex h-5 w-5 items-center justify-center rounded transition-colors hover:bg-accent hover:text-foreground
									{sectionPinned ? 'text-primary/70' : 'text-muted-foreground/40 opacity-0 group-hover/sec:opacity-100'}"
								onclick={() => sectionsStore.togglePinned(group.id!)}
								title={sectionPinned ? $t('projects.unpinSection') : $t('projects.pinSection')}
								aria-label={sectionPinned ? $t('projects.unpinSection') : $t('projects.pinSection')}
							>
								{#if sectionPinned}
									<PinOffIcon class="h-3.5 w-3.5" />
								{:else}
									<PinIcon class="h-3.5 w-3.5" />
								{/if}
							</button>
						{/if}
						<button
							class="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/40 transition-colors hover:bg-accent hover:text-foreground"
							onclick={() => openCreateDialog(group.id)}
							title={$t('task.addTask')}
						>
							<PlusIcon class="h-3.5 w-3.5" />
						</button>
						<div class="h-px flex-1 bg-border/40"></div>
					</div>
					{#if !sectionCollapsed}
						<div class="space-y-px px-1">
							{#each group.tasks as task, i (task.id)}
								<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
									<TaskItem {task} hideWeight={true} />
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		{/if}
	</div>
</div>

<CreateTaskDialog
	bind:open={createDialogOpen}
	projectId={projectId}
	sectionId={createSectionId}
	oncreated={handleTaskCreated}
/>
