<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { Button } from '$lib/components/ui/button';
	import { getApiClient } from '$lib/api/client';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { sections as sectionsApi } from '$lib/api/endpoints/sections';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import type { Project, ProjectSection, Task, TaskInput } from '$lib/api/types';
	import ProjectHeader from '$lib/components/project/ProjectHeader.svelte';
	import SectionList from '$lib/components/project/SectionList.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import ProjectDialog from '$lib/components/dialog/ProjectDialog.svelte';
	import SectionDialog from '$lib/components/dialog/SectionDialog.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const projectId = $derived(Number(page.params.id));

	let project = $state<Project | null>(null);
	let sectionList = $state<ProjectSection[]>([]);
	let quickOpen = $state(false);
	let confirmDeleteOpen = $state(false);
	let confirmSectionOpen = $state(false);
	let pendingSectionDelete = $state<ProjectSection | null>(null);
	let editProjectOpen = $state(false);
	let sectionDialogOpen = $state(false);
	let editingSection = $state<ProjectSection | null>(null);

	const taskList = useListMutator<Task>();
	const mutator = taskList.mutator;

	const tasksWithoutSection = $derived(taskList.items.filter((t) => t.sectionId === null));
	const tasksBySection = $derived.by(() => {
		const map: Record<number, Task[]> = {};
		for (const t of taskList.items) {
			if (t.sectionId !== null) {
				(map[t.sectionId] ??= []).push(t);
			}
		}
		return map;
	});

	const loader = usePageLoad(async (isValid) => {
		const client = getApiClient();
		const [p, sec, ts] = await Promise.all([
			projectsApi.get(client, projectId),
			projectsApi.listSections(client, projectId, { limit: 200 }),
			projectsApi.listTasks(client, projectId, { limit: 500 })
		]);
		if (!isValid()) return;
		project = p;
		sectionList = sec.items;
		taskList.items = ts.items;
	}, { errorMessage: 'Failed to load project', autoLoad: false });

	const actionLabels: Record<string, string> = {
		complete: 'completed', uncomplete: 'uncompleted', cancel: 'cancelled',
		archive: 'archived', unarchive: 'unarchived', pin: 'pinned', unpin: 'unpinned'
	};

	async function action(name: 'complete' | 'uncomplete' | 'cancel' | 'archive' | 'unarchive' | 'pin' | 'unpin') {
		if (!project) return;
		try {
			const client = getApiClient();
			const updated = await projectsApi[name](client, project.id);
			project = updated;
			projectsStore.upsert(updated);
			toast.success(`Project ${actionLabels[name]}`);
		} catch (err) {
			toast.error(describeError(err, `Failed to ${name}`));
		}
	}

	async function onQuickSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		if (!project) return;
		try {
			const client = getApiClient();
			if (target.projectId === null) {
				await tasksApi.createInbox(client, payload);
				toast.success('Task added to inbox');
				return;
			}
			const created = await projectsApi.createTask(client, target.projectId, payload);
			if (target.projectId === project.id) {
				taskList.items = [...taskList.items, created];
			}
			toast.success('Task added');
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

	async function deleteProject() {
		if (!project) return;
		try {
			await projectsApi.remove(getApiClient(), project.id);
			projectsStore.remove(project.id);
			toast.success('Project deleted');
			void goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete project'));
		}
	}

	async function deleteSection() {
		const sec = pendingSectionDelete;
		if (!sec) return;
		try {
			await sectionsApi.remove(getApiClient(), sec.id);
			sectionList = sectionList.filter((s) => s.id !== sec.id);
			taskList.items = taskList.items.map((t) => (t.sectionId === sec.id ? { ...t, sectionId: null } : t));
			toast.success('Section deleted');
			pendingSectionDelete = null;
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete section'));
		}
	}

	function renameSection(sec: ProjectSection) {
		editingSection = sec;
		sectionDialogOpen = true;
	}

	function addSection() {
		editingSection = null;
		sectionDialogOpen = true;
	}

	function onSectionSaved(saved: ProjectSection) {
		const i = sectionList.findIndex((s) => s.id === saved.id);
		sectionList = i >= 0 ? sectionList.map((s) => (s.id === saved.id ? saved : s)) : [...sectionList, saved];
	}

	$effect(() => {
		if (Number.isFinite(projectId)) loader.refetch();
	});

	onMount(() => {
		if (!projectsStore.loaded) projectsStore.load().catch(() => undefined);
	});
</script>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if !project}
	<div class="px-6 py-8 text-sm text-muted-foreground">Project not found</div>
{:else}
	<ProjectHeader
		{project}
		onComplete={() => action('complete')}
		onUncomplete={() => action('uncomplete')}
		onCancel={() => action('cancel')}
		onArchive={() => action('archive')}
		onUnarchive={() => action('unarchive')}
		onPin={() => action('pin')}
		onUnpin={() => action('unpin')}
		onEdit={() => (editProjectOpen = true)}
		onDelete={() => (confirmDeleteOpen = true)}
	/>

	<div class="flex items-center justify-between px-6 py-2">
		<Button size="sm" variant="ghost" onclick={addSection}>
			<PlusIcon class="size-4" />
			Add section
		</Button>
		<Button size="sm" onclick={() => (quickOpen = true)}>
			<PlusIcon class="size-4" />
			Add task
		</Button>
	</div>

	<div class="px-2">
		<ViewContent
			loading={false}
			isEmpty={sectionList.length === 0 && taskList.items.length === 0}
			emptyIcon={FolderIcon}
			emptyTitle="No tasks yet"
			emptyDescription="Add a task or section to start organising this project."
		>
			{#if tasksWithoutSection.length > 0}
				<div class="px-1 py-2">
					<TaskTree
						tasks={tasksWithoutSection}
						showProject={false}
						{mutator}
						onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
					/>
				</div>
			{/if}
			{#if sectionList.length > 0}
				<SectionList
					sections={sectionList}
					{tasksBySection}
					{mutator}
					onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
					onRenameSection={renameSection}
					onRemoveSection={(sec) => {
						pendingSectionDelete = sec;
						confirmSectionOpen = true;
					}}
				/>
			{/if}
		</ViewContent>
	</div>

	<ProjectDialog
		bind:open={editProjectOpen}
		initial={project}
		onSaved={(p) => (project = p)}
	/>
	<SectionDialog
		bind:open={sectionDialogOpen}
		initial={editingSection}
		projectId={project.id}
		onSaved={onSectionSaved}
	/>
	<QuickAddDialog bind:open={quickOpen} onSubmit={onQuickSubmit} defaultProjectId={project.id} />
	<ConfirmDestructiveDialog
		bind:open={confirmDeleteOpen}
		title="Delete project?"
		description="All sections and tasks under this project will be permanently deleted (cascade). This cannot be undone."
		onConfirm={deleteProject}
	/>
	<ConfirmDestructiveDialog
		bind:open={confirmSectionOpen}
		title="Delete section?"
		description="The section will be deleted; tasks in it will be kept and moved to the project root."
		onConfirm={deleteSection}
	/>
{/if}
