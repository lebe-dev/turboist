<script lang="ts">
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { getApiClient } from '$lib/api/client';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import type { Project, SearchResponse, Task } from '$lib/api/types';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import { toggleComplete, describeError } from '$lib/utils/taskActions';
	import { ApiError } from '$lib/api/errors';
	import { onDestroy } from 'svelte';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { t } from '$lib/i18n';
	let q = $state('');
	let active = $state<'tasks' | 'projects'>('tasks');
	let projects = $state<Project[]>([]);
	let total = $state({ tasks: 0, projects: 0 });
	let lastQuery = $state('');

	let timer: ReturnType<typeof setTimeout> | null = null;

	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});

	const taskList = useListMutator<Task>();
	const mutator = taskList.mutator;

	function reset() {
		taskList.items = [];
		projects = [];
		total = { tasks: 0, projects: 0 };
		lastQuery = '';
	}

	const loader = usePageLoad(async (isValid) => {
		const trimmed = q.trim();
		const res: SearchResponse = await viewsApi.search(getApiClient(), {
			q: trimmed,
			type: 'all',
			limit: 100
		});
		if (!isValid()) return;
		lastQuery = trimmed;
		taskList.items = res.tasks?.items ?? [];
		projects = res.projects?.items ?? [];
		total = {
			tasks: res.tasks?.total ?? 0,
			projects: res.projects?.total ?? 0
		};
	}, {
		autoLoad: false,
		onError(err) {
			if (err instanceof ApiError && err.code === 'validation_failed') {
				reset();
				return;
			}
			toast.error(describeError(err, $t('page.search.searchFailed')));
		}
	});

	function onInput(e: Event) {
		q = (e.target as HTMLInputElement).value;
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => {
			if (q.trim().length < 2) {
				loader.cancel();
				reset();
			} else {
				void loader.refetch();
			}
		}, 300);
	}

</script>

<div class="px-6 py-3">
	<div class="relative">
		<MagnifyingGlassIcon
			class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
		/>
		<Input
			value={q}
			oninput={onInput}
			placeholder={$t('page.search.placeholder')}
			class="pl-9"
			autofocus
		/>
	</div>
</div>

<div class="flex items-center gap-1 border-b border-border px-6 pb-2">
	<Button
		size="sm"
		variant={active === 'tasks' ? 'secondary' : 'ghost'}
		onclick={() => (active = 'tasks')}
	>
		{$t('page.search.tasksTab')}
		<Badge variant="outline" class="ml-1 h-4 text-[10px]">{total.tasks}</Badge>
	</Button>
	<Button
		size="sm"
		variant={active === 'projects' ? 'secondary' : 'ghost'}
		onclick={() => (active = 'projects')}
	>
		{$t('page.search.projectsTab')}
		<Badge variant="outline" class="ml-1 h-4 text-[10px]">{total.projects}</Badge>
	</Button>
</div>

<div class="px-2 py-2">
	{#if loader.loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">{$t('page.search.searching')}</div>
	{:else if !lastQuery}
		<EmptyState
			icon={MagnifyingGlassIcon}
			title={$t('page.search.emptyTitle')}
			description={$t('page.search.emptyDescription')}
		/>
	{:else if active === 'tasks'}
		<ViewContent
			loading={false}
			isEmpty={taskList.items.length === 0}
			emptyIcon={MagnifyingGlassIcon}
			emptyTitle={$t('page.search.noTasksMatch')}
		>
			<TaskTree
				tasks={taskList.items}
				{mutator}
				onToggle={(t) => toggleComplete(t, mutator, { removeWhenCompleted: false })}
			/>
		</ViewContent>
	{:else}
		<ViewContent
			loading={false}
			isEmpty={projects.length === 0}
			emptyIcon={FolderIcon}
			emptyTitle={$t('page.search.noProjectsMatch')}
		>
			<ul class="flex flex-col divide-y divide-border/50">
				{#each projects as p (p.id)}
					<li>
						<a
							href={resolve('/(app)/project/[id]', { id: String(p.id) })}
							class="flex items-center gap-3 px-3 py-2 hover:bg-muted/40"
						>
							<span
								class="inline-block size-3 shrink-0 rounded-full"
								style={`background-color: ${p.color}`}
							></span>
							<span class="min-w-0 flex-1 truncate text-sm">{p.title}</span>
							{#if p.status !== 'open'}
								<Badge variant="outline">{$t(`project.status${p.status[0].toUpperCase()}${p.status.slice(1)}`)}</Badge>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		</ViewContent>
	{/if}
</div>
