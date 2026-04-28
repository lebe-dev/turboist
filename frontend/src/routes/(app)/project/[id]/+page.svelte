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
	import { sections as sectionsApi } from '$lib/api/endpoints/sections';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import type { Project, ProjectSection, Task, TaskInput } from '$lib/api/types';
	import ProjectHeader from '$lib/components/project/ProjectHeader.svelte';
	import SectionList from '$lib/components/project/SectionList.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import TaskEditorSheet from '$lib/components/task/TaskEditorSheet.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import ProjectDialog from '$lib/components/dialog/ProjectDialog.svelte';
	import SectionDialog from '$lib/components/dialog/SectionDialog.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		saveEdit,
		describeError
	} from '$lib/utils/taskActions';

	const projectId = $derived(Number(page.params.id));

	let project = $state<Project | null>(null);
	let sectionList = $state<ProjectSection[]>([]);
	let tasks = $state<Task[]>([]);
	let loading = $state(true);
	let quickOpen = $state(false);
	let editing = $state<Task | null>(null);
	let editorOpen = $state(false);
	let confirmDeleteOpen = $state(false);
	let confirmSectionOpen = $state(false);
	let pendingSectionDelete = $state<ProjectSection | null>(null);
	let editProjectOpen = $state(false);
	let sectionDialogOpen = $state(false);
	let editingSection = $state<ProjectSection | null>(null);

	const tasksWithoutSection = $derived(tasks.filter((t) => t.sectionId === null));
	const tasksBySection = $derived.by(() => {
		const map: Record<number, Task[]> = {};
		for (const t of tasks) {
			if (t.sectionId !== null) {
				(map[t.sectionId] ??= []).push(t);
			}
		}
		return map;
	});

	const mutator = {
		replace(t: Task) {
			tasks = tasks.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			tasks = tasks.filter((x) => x.id !== id);
		}
	};

	let requestSeq = 0;

	async function load(): Promise<void> {
		const my = ++requestSeq;
		loading = true;
		try {
			const client = getApiClient();
			const [p, sec, ts] = await Promise.all([
				projectsApi.get(client, projectId),
				projectsApi.listSections(client, projectId, { limit: 200 }),
				projectsApi.listTasks(client, projectId, { limit: 500 })
			]);
			if (my !== requestSeq) return;
			project = p;
			sectionList = sec.items;
			tasks = ts.items;
		} catch (err) {
			if (my !== requestSeq) return;
			toast.error(describeError(err, 'Failed to load project'));
		} finally {
			if (my === requestSeq) loading = false;
		}
	}

	async function action(name: 'complete' | 'uncomplete' | 'cancel' | 'archive' | 'unarchive' | 'pin' | 'unpin') {
		if (!project) return;
		try {
			const client = getApiClient();
			const updated = await projectsApi[name](client, project.id);
			project = updated;
			projectsStore.upsert(updated);
			toast.success(`Project ${name}d`);
		} catch (err) {
			toast.error(describeError(err, `Failed to ${name}`));
		}
	}

	async function onQuickSubmit(payload: TaskInput): Promise<void> {
		if (!project) return;
		try {
			const created = await projectsApi.createTask(getApiClient(), project.id, payload);
			tasks = [...tasks, created];
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
			goto(resolve('/inbox'));
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
			tasks = tasks.map((t) => (t.sectionId === sec.id ? { ...t, sectionId: null } : t));
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

	function openEditor(task: Task): void {
		editing = task;
		editorOpen = true;
	}

	$effect(() => {
		if (Number.isFinite(projectId)) load();
	});

	onMount(() => {
		if (!projectsStore.loaded) projectsStore.load().catch(() => undefined);
	});
</script>

{#if loading}
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
		{#if sectionList.length === 0 && tasks.length === 0}
			<EmptyState
				icon={FolderIcon}
				title="No tasks yet"
				description="Add a task or section to start organising this project."
			/>
		{:else}
			{#if tasksWithoutSection.length > 0}
				<div class="px-1 py-2">
					<TaskTree
						tasks={tasksWithoutSection}
						showProject={false}
						onToggle={(t) => toggleComplete(t, mutator)}
						onPinToggle={(t) => togglePin(t, mutator)}
						onDelete={(t) => deleteTask(t, mutator)}
						onEdit={openEditor}
					/>
				</div>
			{/if}
			{#if sectionList.length > 0}
				<SectionList
					sections={sectionList}
					{tasksBySection}
					onToggle={(t) => toggleComplete(t, mutator)}
					onPinToggle={(t) => togglePin(t, mutator)}
					onDelete={(t) => deleteTask(t, mutator)}
					onEdit={openEditor}
					onRenameSection={renameSection}
					onRemoveSection={(sec) => {
						pendingSectionDelete = sec;
						confirmSectionOpen = true;
					}}
				/>
			{/if}
		{/if}
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
	<TaskEditorSheet
		bind:open={editorOpen}
		task={editing}
		onSubmit={(id, payload) => saveEdit(id, payload, mutator)}
	/>
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
