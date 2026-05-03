<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Input } from '$lib/components/ui/input';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { moveTaskToProject, type ListMutator } from '$lib/utils/taskActions';
	import type { Project, Task } from '$lib/api/types';
	import CheckIcon from 'phosphor-svelte/lib/Check';

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

	$effect(() => {
		if (open) query = '';
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

	async function pick(project: Project) {
		if (!task || submitting) return;
		if (task.projectId === project.id) {
			open = false;
			return;
		}
		submitting = true;
		try {
			await moveTaskToProject(task, project.contextId, project.id, mutator, {
				belongs,
				projectCompleted: project.status === 'completed'
			});
			open = false;
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Move to project</Sheet.Title>
			<Sheet.Description>
				{task ? `Pick a project for "${task.title}"` : ''}
			</Sheet.Description>
		</Sheet.Header>

		<div class="flex flex-col gap-3 overflow-y-auto px-4 py-2">
			<Input placeholder="Search projects…" bind:value={query} autofocus />

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
								onclick={() => pick(project)}
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
					<div class="px-1 py-4 text-sm text-muted-foreground">No projects found.</div>
				{/each}
			</div>
		</div>
	</Sheet.Content>
</Sheet.Root>
