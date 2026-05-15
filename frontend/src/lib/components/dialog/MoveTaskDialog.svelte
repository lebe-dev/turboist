<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import { moveTaskToProject, moveTaskToInbox, type ListMutator } from '$lib/utils/taskActions';
	import type { Project, ProjectSection, Task } from '$lib/api/types';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import TrayIcon from 'phosphor-svelte/lib/Tray';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		task,
		mutator,
		belongs
	}: {
		open?: boolean;
		task: Task | null;
		mutator: ListMutator;
		belongs?: (task: Task) => boolean;
	} = $props();

	let query = $state('');
	let submitting = $state(false);
	let selectedProject = $state<Project | null>(null);
	let sections = $state<ProjectSection[]>([]);
	let loadingSections = $state(false);

	$effect(() => {
		if (open) {
			query = '';
			selectedProject = null;
			sections = [];
		}
	});

	const grouped = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const matches = (p: Project) => !q || p.title.toLowerCase().includes(q);
		const collator = new Intl.Collator(undefined, { sensitivity: 'base' });
		return contextsStore.items
			.map((ctx) => {
				const all = projectsStore.byContext(ctx.id).filter(matches);
				const open = all
					.filter((p) => p.status === 'open')
					.sort((a, b) => collator.compare(a.title, b.title));
				const done = all
					.filter((p) => p.status !== 'open')
					.sort((a, b) => collator.compare(a.title, b.title));
				return { ctx, projects: [...open, ...done] };
			})
			.filter((g) => g.projects.length > 0);
	});

	async function pickProject(project: Project) {
		if (!task || submitting) return;
		loadingSections = true;
		selectedProject = project;
		try {
			const page = await projectsApi.listSections(getApiClient(), project.id, { limit: 200 });
			sections = page.items;
		} finally {
			loadingSections = false;
		}
		if (sections.length === 0) {
			await doMove(project, null);
		}
	}

	async function doMoveToInbox() {
		if (!task || submitting) return;
		submitting = true;
		try {
			await moveTaskToInbox(task, mutator, { belongs });
			open = false;
		} finally {
			submitting = false;
		}
	}

	async function doMove(project: Project, sectionId: number | null) {
		if (!task || submitting) return;
		submitting = true;
		try {
			await moveTaskToProject(task, project.contextId, project.id, mutator, {
				belongs,
				projectCompleted: project.status === 'completed',
				sectionId
			});
			open = false;
		} finally {
			submitting = false;
		}
	}

	function backToProjects() {
		selectedProject = null;
		sections = [];
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			{#if selectedProject}
				<Sheet.Title>{$t('dialog.moveTask.pickSectionTitle')}</Sheet.Title>
				<Sheet.Description>
					{$t('dialog.moveTask.pickSectionDescription', { values: { project: selectedProject.title } })}
				</Sheet.Description>
			{:else}
				<Sheet.Title>{$t('dialog.moveTask.title')}</Sheet.Title>
				<Sheet.Description>
					{task ? $t('dialog.moveTask.description', { values: { title: task.title } }) : ''}
				</Sheet.Description>
			{/if}
		</Sheet.Header>

		<div class="flex flex-col gap-3 overflow-y-auto px-4 py-2">
			{#if selectedProject}
				<!-- Section picker step -->
				<Button
					variant="ghost"
					size="sm"
					class="w-fit gap-1.5 px-2 text-muted-foreground"
					onclick={backToProjects}
					disabled={submitting}
				>
					<ArrowLeftIcon class="size-3.5" />
					{$t('dialog.moveTask.backToProjects')}
				</Button>

				{#if loadingSections}
					<div class="px-1 py-4 text-sm text-muted-foreground">{$t('dialog.moveTask.loadingSections')}</div>
				{:else}
					<div class="flex flex-col gap-1">
						<button
							type="button"
							disabled={submitting}
							onclick={() => doMove(selectedProject!, null)}
							class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
							class:bg-accent={task?.sectionId === null && task?.projectId === selectedProject?.id}
						>
							<span class="text-muted-foreground">{$t('dialog.moveTask.noSection')}</span>
							{#if task?.sectionId === null && task?.projectId === selectedProject?.id}
								<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
							{/if}
						</button>
						{#each sections as section (section.id)}
							{@const active = task?.sectionId === section.id}
							<button
								type="button"
								disabled={submitting}
								onclick={() => doMove(selectedProject!, section.id)}
								class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
								class:bg-accent={active}
							>
								<span>{section.title}</span>
								{#if active}
									<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			{:else}
				<!-- Project picker step -->
				<Input placeholder={$t('dialog.moveTask.searchPlaceholder')} bind:value={query} autofocus />

				{@const inInbox = task?.inboxId !== null}
				<button
					type="button"
					disabled={submitting || inInbox}
					onclick={doMoveToInbox}
					class="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
					class:bg-accent={inInbox}
				>
					<TrayIcon class="size-3.5 shrink-0 text-muted-foreground" />
					<span class="flex-1">{$t('nav.inbox')}</span>
					{#if inInbox}
						<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
					{/if}
				</button>

				<div class="border-t border-border/40"></div>

				<div class="flex flex-col gap-3">
					{#each grouped as group (group.ctx.id)}
						<div class="flex flex-col gap-1">
							<div class="px-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
								{group.ctx.name}
							</div>
							{#each group.projects as project (project.id)}
								{@const active = task?.projectId === project.id}
								{@const done = project.status !== 'open'}
								<button
									type="button"
									disabled={submitting}
									onclick={() => pickProject(project)}
									class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
									class:bg-accent={active}
								>
									<span class="flex items-center gap-2">
										{#if done}
											<CheckIcon class="size-3 text-muted-foreground" weight="bold" />
										{:else}
											<span
												class="inline-block size-2.5 rounded-full"
												style="background-color: {project.color};"
											></span>
										{/if}
										<span class:text-muted-foreground={done}>{project.title}</span>
									</span>
									{#if active}
										<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
									{/if}
								</button>
							{/each}
						</div>
					{:else}
						<div class="px-1 py-4 text-sm text-muted-foreground">{$t('dialog.moveTask.noProjects')}</div>
					{/each}
				</div>
			{/if}
		</div>
	</Sheet.Content>
</Sheet.Root>
