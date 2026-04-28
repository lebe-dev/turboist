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
	import { ApiError } from '$lib/api/errors';
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import type { Context, Project, Task, TaskInput } from '$lib/api/types';
	import ContextHeader from '$lib/components/context/ContextHeader.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const contextId = $derived(Number(page.params.id));

	let context = $state<Context | null>(null);
	let notFound = $state(false);
	let projects = $state<Project[]>([]);
	let activeProjectId = $state<number | 'all'>('all');
	let quickOpen = $state(false);
	let confirmDeleteOpen = $state(false);
	let editOpen = $state(false);

	const taskList = useListMutator<Task>();
	const mutator = taskList.mutator;

	const filteredTasks = $derived(
		activeProjectId === 'all'
			? taskList.items
			: taskList.items.filter((t) => t.projectId === activeProjectId)
	);

	const loader = usePageLoad(async (isValid) => {
		context = null;
		notFound = false;
		projects = [];
		taskList.items = [];
		if (!Number.isFinite(contextId)) return;
		const client = getApiClient();
		const [c, projs, ts] = await Promise.all([
			contextsApi.get(client, contextId),
			contextsApi.listProjects(client, contextId, { limit: 200 }),
			contextsApi.listTasks(client, contextId, { limit: 500 })
		]);
		if (!isValid()) return;
		context = c;
		projects = projs.items;
		taskList.items = ts.items;
		activeProjectId = 'all';
	}, {
		errorMessage: 'Failed to load context',
		autoLoad: false,
		initialLoading: true,
		onError(err) {
			if (err instanceof ApiError && err.code === 'not_found') notFound = true;
		}
	});

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
			void goto(resolve('/inbox'));
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
				taskList.items = [...taskList.items, created];
				toast.success('Task added');
				return;
			}
			const targetInContext = projects.some((p) => p.id === target.projectId);
			const created = await projectsApi.createTask(client, target.projectId, payload);
			if (targetInContext) {
				taskList.items = [...taskList.items, created];
			}
			toast.success('Task added');
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

	$effect(() => {
		if (Number.isFinite(contextId)) void loader.refetch();
	});

	onMount(() => {
		if (!projectsStore.loaded) projectsStore.load().catch(() => undefined);
	});
</script>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !context}
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
				All ({taskList.items.length})
			</Button>
			{#each projects as p (p.id)}
				{@const count = taskList.items.filter((t) => t.projectId === p.id).length}
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
		<ViewContent
			loading={false}
			isEmpty={filteredTasks.length === 0}
			emptyIcon={FolderIcon}
			emptyTitle="No tasks"
			emptyDescription="No tasks yet for this filter."
		>
			<TaskTree
				tasks={filteredTasks}
				{mutator}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
			/>
		</ViewContent>
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
