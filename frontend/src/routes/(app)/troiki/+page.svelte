<script lang="ts">
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import PlayIcon from 'phosphor-svelte/lib/Play';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import InfoIcon from 'phosphor-svelte/lib/Info';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import type { Task, TaskInput, TroikiCategory, TroikiProject, TroikiSlot } from '$lib/api/types';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { Button } from '$lib/components/ui/button';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { followUpStore } from '$lib/stores/followUp.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { t } from '$lib/i18n';
	import type { ListMutator } from '$lib/utils/taskActions';

	async function loadAll(): Promise<void> {
		await troikiStore.load();
	}

	const loader = usePageLoad(loadAll, { errorMessage: $t('troiki.toast.loadFailed') });

	const view = $derived(troikiStore.value);

	const sections: Array<{ key: TroikiCategory; labelKey: string; descriptionKey: string }> = [
		{
			key: 'important',
			labelKey: 'troiki.section.important',
			descriptionKey: 'troiki.section.importantDescription'
		},
		{
			key: 'medium',
			labelKey: 'troiki.section.medium',
			descriptionKey: 'troiki.section.mediumDescription'
		},
		{
			key: 'rest',
			labelKey: 'troiki.section.rest',
			descriptionKey: 'troiki.section.restDescription'
		}
	];

	function slotFor(key: TroikiCategory): TroikiSlot {
		const slot = view[key];
		if (!settingsStore.publicView) return slot;
		return {
			...slot,
			projects: slot.projects.filter((p) => !p.isPrivate)
		};
	}

	function projectMutator(project: TroikiProject): ListMutator {
		return {
			replace(task: Task) {
				troikiStore.applyTaskUpdate(task);
			},
			remove(id: number) {
				troikiStore.removeTask(id);
				void project;
			}
		};
	}

	async function onTaskToggle(task: Task): Promise<void> {
		const client = getApiClient();
		const wasOpen = task.status !== 'completed';
		try {
			const updated = wasOpen
				? await tasksApi.complete(client, task.id)
				: await tasksApi.uncomplete(client, task.id);
			// Completing a task can grow Medium/Rest capacity; refetch to get fresh slots.
			if (updated.status === 'completed' || task.status === 'completed') {
				await troikiStore.load();
			} else {
				troikiStore.applyTaskUpdate(updated);
			}
			if (wasOpen && updated.status === 'completed' && !updated.recurrenceRule) {
				followUpStore.push(updated, async () => {
					await tasksApi.uncomplete(client, task.id);
					await troikiStore.load();
				});
			}
		} catch (err) {
			toast.error(describeError(err, $t('troiki.toast.updateFailed')));
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

	function sectionLabel(category: TroikiCategory): string {
		return $t(`troiki.section.${category}`);
	}

	async function onAddSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		if (target.projectId === null) {
			toast.error($t('troiki.toast.pickProject'));
			return;
		}
		const client = getApiClient();
		try {
			await projectsApi.createTask(client, target.projectId, payload);
			toast.success(
				$t('troiki.toast.addedTo', { values: { section: sectionLabel(addCategory) } })
			);
		} catch (err) {
			toast.error(describeError(err, $t('troiki.toast.createFailed')));
			return;
		}
		await troikiStore.load();
	}

	async function startSystem(): Promise<void> {
		if (!canStart || starting) return;
		starting = true;
		try {
			await troikiStore.start();
			toast.success($t('troiki.toast.started'));
		} catch (err) {
			toast.error(describeError(err, $t('troiki.toast.startFailed')));
		} finally {
			starting = false;
		}
	}
</script>

<div class="px-2 py-2">
	<div class="flex items-center justify-between px-3 pt-2 pb-4">
		<h1 class="text-2xl font-bold tracking-tight">{$t('topbar.troikiSystem')}</h1>
		<HoverCard.Root>
			<HoverCard.Trigger>
				<button
					type="button"
					class="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
					aria-label={$t('troiki.howAria')}
				>
					<InfoIcon class="size-3.5" />
					{$t('troiki.howIt')}
				</button>
			</HoverCard.Trigger>
			<HoverCard.Content align="end" class="w-96 text-xs/relaxed">
				<p class="mb-2 font-semibold text-foreground">{$t('troiki.rulesTitle')}</p>
				<p class="mb-2 text-muted-foreground">{$t('troiki.rulesIntro')}</p>
				<div class="mb-2 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5">
					<span class="font-medium text-foreground">{$t('troiki.section.important')}</span>
					<span class="text-muted-foreground">{$t('troiki.rules.importantHint')}</span>
					<span class="font-medium text-foreground">{$t('troiki.section.medium')}</span>
					<span class="text-muted-foreground">{$t('troiki.rules.mediumHint')}</span>
					<span class="font-medium text-foreground">{$t('troiki.section.rest')}</span>
					<span class="text-muted-foreground">{$t('troiki.rules.restHint')}</span>
				</div>
				<ul class="space-y-0.5 text-muted-foreground">
					<li>• {$t('troiki.rules.bullet1')}</li>
					<li>• {$t('troiki.rules.bullet2')}</li>
					<li>• {$t('troiki.rules.bullet3')}</li>
					<li>• {$t('troiki.rules.bullet4')}</li>
				</ul>
			</HoverCard.Content>
		</HoverCard.Root>
	</div>
	{#if loader.loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">{$t('app.loading')}</div>
	{:else}
		<header class="flex items-center justify-between px-3 pb-1">
			<div class="text-xs text-muted-foreground">
				{#if view.started}
					{$t('troiki.cycleInProgress')}
				{:else}
					{$t('troiki.initialFill')}
				{/if}
			</div>
			{#if !view.started}
				<Button
					size="sm"
					variant="default"
					disabled={!canStart || starting}
					onclick={startSystem}
					title={canStart ? $t('troiki.startEnabled') : $t('troiki.startDisabled')}
				>
					<PlayIcon class="size-4" weight="fill" />
					{starting ? $t('troiki.starting') : $t('troiki.start')}
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
								{$t(section.labelKey)}
							</h2>
							{#if locked}
								<span
									class="inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									aria-label={$t('troiki.locked')}
									title={$t('troiki.lockedTitle')}
								>
									<LockSimpleIcon class="size-3" />
									<span>{$t('troiki.locked')}</span>
								</span>
							{:else if initialMode}
								<span
									class="rounded-full border border-dashed border-border bg-muted/20 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									title={$t('troiki.openInitialTitle')}
								>
									{$t('troiki.openInitial', { values: { count: open } })}
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
							<p class="hidden text-xs text-muted-foreground sm:block">
								{$t(section.descriptionKey)}
							</p>
						</div>
					</header>

					{#if locked}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/20 px-3 py-4 text-xs text-muted-foreground"
						>
							{#if section.key === 'medium'}
								{$t('troiki.unlockMedium')}
							{:else}
								{$t('troiki.unlockRest')}
							{/if}
						</div>
					{:else if open === 0 && initialMode}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/10 px-3 py-4 text-xs text-muted-foreground"
						>
							{$t('troiki.assignHint')}
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
											<a
												href={resolve(`/project/${project.id}`)}
												class="truncate text-sm font-medium hover:underline">{project.title}</a
											>
											<span
												class="text-[11px] tabular-nums text-muted-foreground"
												title={$t('troiki.openTasksTitle')}
											>
												{project.tasks.filter((tk) => tk.status === 'open').length}
											</span>
										</div>
										<Button
											size="sm"
											variant="ghost"
											class="h-7 px-2 text-xs"
											onclick={() => openAdd(section.key, project.id)}
											aria-label={$t('troiki.addTaskAria', { values: { name: project.title } })}
										>
											<PlusIcon class="size-3.5" />
											{$t('troiki.addTask')}
										</Button>
									</div>
									{#if project.tasks.length > 0}
										<TaskTree
											tasks={project.tasks}
											showProject={false}
											hideDue
											mutator={projectMutator(project)}
											onToggle={(tk) => void onTaskToggle(tk)}
										/>
									{:else}
										<div class="px-3 py-3 text-xs text-muted-foreground">
											{$t('troiki.noTasks')}
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
									<span>{$t('troiki.emptySlot')}</span>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			{/each}
		</div>
	{/if}
</div>

<QuickAddDialog bind:open={addOpen} defaultProjectId={addProjectId} onSubmit={onAddSubmit} />
