<script lang="ts">
	import { toast } from 'svelte-sonner';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import PlayIcon from 'phosphor-svelte/lib/Play';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import type { Task, TaskInput, TroikiCategory, TroikiProject, TroikiSlot } from '$lib/api/types';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { Button } from '$lib/components/ui/button';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import type { ListMutator } from '$lib/utils/taskActions';

	async function loadAll(): Promise<void> {
		await troikiStore.load();
	}

	const loader = usePageLoad(loadAll, { errorMessage: 'Failed to load Troiki' });

	const view = $derived(troikiStore.value);

	const sections: Array<{ key: TroikiCategory; label: string; description: string }> = [
		{ key: 'important', label: 'Important', description: 'Top three projects that demand focus.' },
		{ key: 'medium', label: 'Medium', description: 'Earned by completing Important tasks.' },
		{ key: 'rest', label: 'Rest', description: 'Earned by completing Medium tasks.' }
	];

	function slotFor(key: TroikiCategory): TroikiSlot {
		return view[key];
	}

	function projectMutator(project: TroikiProject): ListMutator {
		return {
			replace(t: Task) {
				troikiStore.applyTaskUpdate(t);
			},
			remove(id: number) {
				troikiStore.removeTask(id);
				void project;
			}
		};
	}

	async function onTaskToggle(t: Task): Promise<void> {
		const client = getApiClient();
		try {
			const updated =
				t.status === 'completed'
					? await tasksApi.uncomplete(client, t.id)
					: await tasksApi.complete(client, t.id);
			// Completing a task can grow Medium/Rest capacity; refetch to get fresh slots.
			if (updated.status === 'completed' || t.status === 'completed') {
				await troikiStore.load();
			} else {
				troikiStore.applyTaskUpdate(updated);
			}
		} catch (err) {
			toast.error(describeError(err, 'Failed to update task'));
		}
	}

	let starting = $state(false);
	const canStart = $derived(!view.started && view.important.projects.length > 0);

	let addOpen = $state(false);
	let addCategory = $state<TroikiCategory>('important');
	let addProjectId = $state<number | null>(null);

	function openAdd(category: TroikiCategory, projectId: number): void {
		addCategory = category;
		addProjectId = projectId;
		addOpen = true;
	}

	async function onAddSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		if (target.projectId === null) {
			toast.error('Pick a project in this Troiki section');
			return;
		}
		const client = getApiClient();
		try {
			await projectsApi.createTask(client, target.projectId, payload);
			toast.success(`Task added to ${addCategory}`);
		} catch (err) {
			toast.error(describeError(err, 'Failed to create task'));
			return;
		}
		await troikiStore.load();
	}

	async function startSystem(): Promise<void> {
		if (!canStart || starting) return;
		starting = true;
		try {
			await troikiStore.start();
			toast.success('Troiki started');
		} catch (err) {
			toast.error(describeError(err, 'Failed to start Troiki'));
		} finally {
			starting = false;
		}
	}
</script>

<div class="px-2 py-2">
	{#if loader.loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else}
		<header class="flex items-center justify-between px-3 pb-1">
			<div class="text-xs text-muted-foreground">
				{#if view.started}
					Cycle in progress — Medium and Rest unlock as you complete the previous category.
				{:else}
					Initial fill — assign projects to Important, then optionally seed Medium and Rest. Press
					Start to lock in the cycle.
				{/if}
			</div>
			{#if !view.started}
				<Button
					size="sm"
					variant="default"
					disabled={!canStart || starting}
					onclick={startSystem}
					title={canStart
						? 'Start the Troiki cycle'
						: 'Assign at least one project to Important first'}
				>
					<PlayIcon class="size-4" weight="fill" />
					{starting ? 'Starting…' : 'Start the system'}
				</Button>
			{/if}
		</header>
		<div class="flex flex-col gap-6 py-2">
			{#each sections as section (section.key)}
				{@const slot = slotFor(section.key)}
				{@const initialMode = !view.started && (section.key === 'medium' || section.key === 'rest')}
				{@const locked = !initialMode && slot.capacity === 0}
				{@const open = slot.projects.length}
				{@const cap = slot.capacity}
				{@const emptySlots = Math.max(0, cap - open)}
				<section>
					<header class="flex items-baseline justify-between px-3 pb-2">
						<div class="flex items-center gap-2">
							<h2 class="text-sm font-semibold uppercase tracking-wide text-foreground">
								{section.label}
							</h2>
							{#if locked}
								<span
									class="inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									aria-label="Locked"
									title="Locked — earn capacity by completing the previous category"
								>
									<LockSimpleIcon class="size-3" />
									<span>Locked</span>
								</span>
							{:else if initialMode}
								<span
									class="rounded-full border border-dashed border-border bg-muted/20 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									title="Open during initial fill — capacity is locked in when you press Start"
								>
									Open · {open}
								</span>
							{:else}
								<span
									class="rounded-full border border-border bg-muted/30 px-2 py-0.5 text-[11px] tabular-nums text-muted-foreground"
								>
									{open}/{cap}
								</span>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							<p class="hidden text-xs text-muted-foreground sm:block">{section.description}</p>
						</div>
					</header>

					{#if locked}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/20 px-3 py-4 text-xs text-muted-foreground"
						>
							{#if section.key === 'medium'}
								Complete an Important task to unlock a Medium slot.
							{:else}
								Complete a Medium task to unlock a Rest slot.
							{/if}
						</div>
					{:else if open === 0 && initialMode}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/10 px-3 py-4 text-xs text-muted-foreground"
						>
							Use the project actions menu (⋯ → Assign to Troiki) to assign projects to this
							section before starting the cycle.
						</div>
					{:else}
						<div class="flex flex-col gap-3">
							{#each slot.projects as project (project.id)}
								<div class="rounded-md border border-border/60 bg-muted/10">
									<div
										class="flex items-center justify-between gap-2 border-b border-border/40 px-3 py-2"
									>
										<div class="flex min-w-0 items-center gap-2">
											<span
												class="inline-block size-2.5 shrink-0 rounded-full"
												style={`background-color: ${project.color}`}
												aria-hidden="true"
											></span>
											<span class="truncate text-sm font-medium">{project.title}</span>
											<span
												class="text-[11px] tabular-nums text-muted-foreground"
												title="Open tasks in this project"
											>
												{project.tasks.filter((t) => t.status === 'open').length}
											</span>
										</div>
										<Button
											size="sm"
											variant="ghost"
											class="h-7 px-2 text-xs"
											onclick={() => openAdd(section.key, project.id)}
											aria-label={`Add task to ${project.title}`}
										>
											<PlusIcon class="size-3.5" />
											Add task
										</Button>
									</div>
									{#if project.tasks.length > 0}
										<TaskTree
											tasks={project.tasks}
											showProject={false}
											hideDue
											mutator={projectMutator(project)}
											onToggle={(t) => void onTaskToggle(t)}
										/>
									{:else}
										<div class="px-3 py-3 text-xs text-muted-foreground">
											No tasks yet — add one to get started.
										</div>
									{/if}
								</div>
							{/each}
							{#each Array.from({ length: emptySlots }) as _, i (i)}
								<div
									class="flex items-center gap-3 rounded-lg border border-dashed border-border/40 px-3 py-3 text-xs text-muted-foreground/70"
								>
									<span
										class="inline-block size-3 shrink-0 rounded-full border border-dashed border-border/70"
									></span>
									<span>Empty slot — assign a project</span>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			{/each}
		</div>
	{/if}
</div>

<QuickAddDialog
	bind:open={addOpen}
	defaultProjectId={addProjectId}
	onSubmit={onAddSubmit}
/>
