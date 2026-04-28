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
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import type { Context, Project, Task, TaskInput } from '$lib/api/types';
	import ContextHeader from '$lib/components/context/ContextHeader.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		describeError
	} from '$lib/utils/taskActions';

	const contextId = $derived(Number(page.params.id));

	let context = $state<Context | null>(null);
	let projects = $state<Project[]>([]);
	let tasks = $state<Task[]>([]);
	let activeProjectId = $state<number | 'all'>('all');
	let loading = $state(true);
	let quickOpen = $state(false);
	let confirmDeleteOpen = $state(false);
	let editOpen = $state(false);

	const filteredTasks = $derived(
		activeProjectId === 'all'
			? tasks
			: tasks.filter((t) => t.projectId === activeProjectId)
	);

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
			const [c, projs, ts] = await Promise.all([
				contextsApi.get(client, contextId),
				contextsApi.listProjects(client, contextId, { limit: 200 }),
				contextsApi.listTasks(client, contextId, { limit: 500 })
			]);
			if (my !== requestSeq) return;
			context = c;
			projects = projs.items;
			tasks = ts.items;
			activeProjectId = 'all';
		} catch (err) {
			if (my !== requestSeq) return;
			toast.error(describeError(err, 'Failed to load context'));
		} finally {
			if (my === requestSeq) loading = false;
		}
	}

	async function toggleFavourite() {
		if (!context) return;
		try {
			const updated = await contextsApi.update(getApiClient(), context.id, {
				isFavourite: !context.isFavourite
			});
			context = updated;
			contextsStore.upsert(updated);
		} catch (err) {
			toast.error(describeError(err, 'Failed to update context'));
		}
	}

	async function deleteContext() {
		if (!context) return;
		try {
			await contextsApi.remove(getApiClient(), context.id);
			contextsStore.remove(context.id);
			projectsStore.items
				.filter((p) => p.contextId === context!.id)
				.forEach((p) => projectsStore.remove(p.id));
			toast.success('Context deleted');
			goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete context'));
		}
	}

	async function onQuickSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		if (!context) return;
		try {
			const client = getApiClient();
			if (target.projectId === null) {
				const created = await contextsApi.createTask(client, context.id, payload);
				tasks = [...tasks, created];
				toast.success('Task added');
				return;
			}
			const targetInContext = projects.some((p) => p.id === target.projectId);
			const created = await projectsApi.createTask(client, target.projectId, payload);
			if (targetInContext) {
				tasks = [...tasks, created];
			}
			toast.success('Task added');
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

	$effect(() => {
		if (Number.isFinite(contextId)) load();
	});

	onMount(() => {
		if (!projectsStore.loaded) projectsStore.load().catch(() => undefined);
	});
</script>

{#if loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if !context}
	<div class="px-6 py-8 text-sm text-muted-foreground">Context not found</div>
{:else}
	<ContextHeader
		{context}
		onEdit={() => (editOpen = true)}
		onToggleFavourite={toggleFavourite}
		onDelete={() => (confirmDeleteOpen = true)}
	/>

	<div class="flex items-center justify-between gap-2 px-6 py-2">
		<div class="flex flex-wrap items-center gap-1 overflow-x-auto">
			<Button
				size="sm"
				variant={activeProjectId === 'all' ? 'secondary' : 'ghost'}
				onclick={() => (activeProjectId = 'all')}
			>
				All ({tasks.length})
			</Button>
			{#each projects as p (p.id)}
				{@const count = tasks.filter((t) => t.projectId === p.id).length}
				<Button
					size="sm"
					variant={activeProjectId === p.id ? 'secondary' : 'ghost'}
					onclick={() => (activeProjectId = p.id)}
				>
					<span
						class="inline-block size-2 rounded-full"
						style={`background-color: ${p.color}`}
					></span>
					{p.title} ({count})
				</Button>
			{/each}
		</div>
		<Button size="sm" onclick={() => (quickOpen = true)}>
			<PlusIcon class="size-4" />
			Add task
		</Button>
	</div>

	<div class="px-2">
		{#if filteredTasks.length === 0}
			<EmptyState
				icon={FolderIcon}
				title="No tasks"
				description="No tasks yet for this filter."
			/>
		{:else}
			<TaskTree
				tasks={filteredTasks}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
				onPinToggle={(t) => togglePin(t, mutator)}
				onDelete={(t) => deleteTask(t, mutator)}
			/>
		{/if}
	</div>

	<QuickAddDialog bind:open={quickOpen} emptyProjectLabel="No project" onSubmit={onQuickSubmit} />
	<ContextDialog
		bind:open={editOpen}
		initial={context}
		onSaved={(c) => (context = c)}
	/>
	<ConfirmDestructiveDialog
		bind:open={confirmDeleteOpen}
		title="Delete context?"
		description="All projects, sections, and tasks under this context will be permanently deleted (cascade). This cannot be undone."
		onConfirm={deleteContext}
	/>
{/if}
