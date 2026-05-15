<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import { moveTaskToSection, type ListMutator } from '$lib/utils/taskActions';
	import { toast } from 'svelte-sonner';
	import { describeError } from '$lib/utils/taskActions';
	import type { ProjectSection, Task } from '$lib/api/types';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		task,
		mutator,
		belongs,
		sections: sectionsProp
	}: {
		open?: boolean;
		task: Task | null;
		mutator: ListMutator;
		belongs?: (task: Task) => boolean;
		sections?: ProjectSection[];
	} = $props();

	let sections = $state<ProjectSection[]>([]);
	let loading = $state(false);
	let submitting = $state(false);

	$effect(() => {
		if (!open || !task?.projectId) return;
		if (sectionsProp) {
			sections = sectionsProp;
			return;
		}
		loading = true;
		projectsApi
			.listSections(getApiClient(), task.projectId, { limit: 200 })
			.then((page) => {
				sections = page.items;
			})
			.catch((err) => {
				toast.error(describeError(err, $t('dialog.moveSection.failedLoad')));
			})
			.finally(() => {
				loading = false;
			});
	});

	async function pick(sectionId: number | null) {
		if (!task || submitting) return;
		const projectId = task.projectId;
		const contextId = task.contextId;
		if (projectId === null || contextId === null) return;
		submitting = true;
		try {
			await moveTaskToSection(task, contextId, projectId, sectionId, mutator, { belongs });
			open = false;
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{$t('dialog.moveSection.title')}</Sheet.Title>
			<Sheet.Description>
				{task ? $t('dialog.moveSection.description', { values: { title: task.title } }) : ''}
			</Sheet.Description>
		</Sheet.Header>

		<div class="flex flex-col gap-2 overflow-y-auto px-4 py-2">
			{#if loading}
				<div class="px-1 py-4 text-sm text-muted-foreground">{$t('dialog.moveSection.loading')}</div>
			{:else if sections.length === 0}
				<div class="px-1 py-4 text-sm text-muted-foreground">{$t('dialog.moveSection.noSections')}</div>
			{:else}
				<button
					type="button"
					disabled={submitting}
					onclick={() => pick(null)}
					class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
					class:bg-accent={task?.sectionId === null}
				>
					<span class="text-muted-foreground">{$t('dialog.moveSection.noSection')}</span>
					{#if task?.sectionId === null}
						<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
					{/if}
				</button>
				{#each sections as section (section.id)}
					{@const active = task?.sectionId === section.id}
					<button
						type="button"
						disabled={submitting}
						onclick={() => pick(section.id)}
						class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
						class:bg-accent={active}
					>
						<span>{section.title}</span>
						{#if active}
							<CheckIcon class="size-4 text-muted-foreground" weight="bold" />
						{/if}
					</button>
				{/each}
			{/if}
		</div>
	</Sheet.Content>
</Sheet.Root>
