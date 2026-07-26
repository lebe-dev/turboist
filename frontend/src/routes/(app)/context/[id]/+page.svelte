<script lang="ts">
	import { onMount, untrack } from 'svelte';
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
	import { settingsStore } from '$lib/stores/settings.svelte';
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
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import { t } from '$lib/i18n';


	const contextId = $derived(Number(page.params.id));

	let context = $state<Context | null>(null);
	$effect(() => { if (context) viewFilterStore.setTitle(context.name); });
	let notFound = $state(false);
	let projects = $state<Project[]>([]);
	let activeProjectId = $state<number | 'all'>('all');
	const visibleProjects = $derived(
		settingsStore.publicView ? projects.filter((p) => !p.isPrivate) : projects
	);
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

	// Nothing is cleared before the await: a background revalidation runs this same
	// fetcher, and blanking `context` mid-flight made the template fall through to
	// "context not found" and destroy the whole page — every SSE event flashed the
	// list away and swallowed whatever the user was clicking. Navigation is already
	// covered by `loading`, which the template renders as a spinner. The project
	// filter is reset on navigation only (see the contextId effect), not on every
	// refresh.
	const loader = usePageLoad(async (isValid) => {
		if (!Number.isFinite(contextId)) {
			context = null;
			notFound = false;
			projects = [];
			taskList.setFromServer([]);
			return;
		}
		const client = getApiClient();
		const [c, projs, ts] = await Promise.all([
			contextsApi.get(client, contextId),
			contextsApi.listProjects(client, contextId, { limit: 200 }),
			contextsApi.listTasks(client, contextId, { limit: 500 })
		]);
		if (!isValid()) return;
		notFound = false;
		context = c;
		projects = projs.items;
		taskList.setFromServer(ts.items);
	}, {
		errorMessage: $t('page.context.errorLoading'),
		autoLoad: false,
		initialLoading: true,
		epoch: () => taskList.epoch,
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
			toast.error(describeError(err, $t('page.context.failedUpdate')));
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
			toast.success($t('page.context.deleted'));
			void goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, $t('page.context.failedDelete')));
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
				toast.success($t('page.context.taskAdded'));
				return;
			}
			const targetInContext = projects.some((p) => p.id === target.projectId);
			const created = await projectsApi.createTask(client, target.projectId, payload);
			if (targetInContext) {
				taskList.items = [...taskList.items, created];
			}
			toast.success('Task added');
		} catch (err) {
			toast.error(describeError(err, $t('page.context.failedAddTask')));
		}
	}

	$effect(() => {
		if (!Number.isFinite(contextId)) return;
		// A different context means a different project set, so drop the filter.
		activeProjectId = 'all';
		untrack(() => void loader.refetch());
	});

	useInvalidation(['tasks'], () => void loader.revalidate());

	onMount(() => {
		if (!projectsStore.loaded) projectsStore.load().catch(() => undefined);
	});
</script>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('app.loading')}</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !context}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('page.context.notFound')}</div>
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
				{$t('page.context.all', { values: { count: taskList.items.length } })}
			</Button>
			{#each visibleProjects as p (p.id)}
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
			{$t('task.addTask')}
		</Button>
	</div>

	<div class="px-2">
		<ViewContent
			loading={false}
			isEmpty={filteredTasks.length === 0}
			emptyIcon={FolderIcon}
			emptyTitle={$t('page.context.emptyTitle')}
			emptyDescription={$t('page.context.emptyDescription')}
		>
			<TaskTree
				tasks={filteredTasks}
				{mutator}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
			/>
		</ViewContent>
	</div>

	<QuickAddDialog bind:open={quickOpen} emptyProjectLabel={$t('page.context.noProject')} onSubmit={onQuickSubmit} />
	<ContextDialog
		bind:open={editOpen}
		initial={context}
		onSaved={(c) => (context = c)}
	/>
	<ConfirmDestructiveDialog
		bind:open={confirmDeleteOpen}
		title={$t('page.context.confirmDeleteTitle')}
		description={$t('page.context.confirmDeleteDesc')}
		onConfirm={deleteContext}
	/>
{/if}
