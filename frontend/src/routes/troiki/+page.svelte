<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import type { SectionClass, Task } from '$lib/api/types';
	import TaskItem from '$lib/components/TaskItem.svelte';
	import Layers3Icon from '@lucide/svelte/icons/layers-3';
	import LockIcon from '@lucide/svelte/icons/lock';
	import LockOpenIcon from '@lucide/svelte/icons/lock-open';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import { t } from 'svelte-intl-precompile';
	import { toast } from 'svelte-sonner';

	let addInputs = $state<Record<SectionClass, string>>({
		important: '',
		medium: '',
		rest: ''
	});
	let submitting = $state<Record<SectionClass, boolean>>({
		important: false,
		medium: false,
		rest: false
	});
	let completedExpanded = $state<Record<SectionClass, boolean>>({
		important: false,
		medium: false,
		rest: false
	});

	onMount(() => {
		tasksStore.stop();
		troikiStore.enter();
	});

	onDestroy(() => {
		troikiStore.exit();
		tasksStore.start();
	});

	async function handleAddTask(sectionClass: SectionClass) {
		const content = addInputs[sectionClass].trim();
		if (!content || submitting[sectionClass]) return;

		submitting[sectionClass] = true;
		try {
			await troikiStore.addTask(sectionClass, content, '');
			addInputs[sectionClass] = '';
		} catch {
			toast.error($t('errors.createFailed'));
		} finally {
			submitting[sectionClass] = false;
		}
	}

	function handleKeydown(e: KeyboardEvent, sectionClass: SectionClass) {
		if (e.key === 'Enter') {
			e.preventDefault();
			handleAddTask(sectionClass);
		}
	}

	function previousSectionName(sectionClass: SectionClass): string {
		const idx = troikiStore.sections.findIndex((s) => s.class === sectionClass);
		if (idx > 0) return troikiStore.sections[idx - 1].name;
		return '';
	}

	function sectionColor(sectionClass: SectionClass): string {
		switch (sectionClass) {
			case 'important': return 'text-amber-500';
			case 'medium': return 'text-blue-500';
			default: return 'text-emerald-500';
		}
	}

	function completedForSection(sectionClass: SectionClass): Task[] {
		return troikiStore.completedSections.find((s) => s.class === sectionClass)?.tasks ?? [];
	}
</script>

{#snippet taskRow(task: Task, textSize: string)}
	<TaskItem {task} {textSize} hideDecompose={true} hidePriority={true} />
{/snippet}

<div class="flex h-full flex-col">
	<header class="flex h-12 shrink-0 items-center gap-2.5 border-b border-border/50 px-4 md:px-6">
		<Layers3Icon class="h-4 w-4 shrink-0 text-muted-foreground" />
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{$t('troiki.title')}</h1>
	</header>

	{#if troikiStore.loading}
		<div class="flex flex-1 items-center justify-center">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
		</div>
	{:else}
		<div class="flex-1 overflow-y-auto px-1 pb-20 pt-2 md:px-3 md:py-3">
			{#each troikiStore.sections as section (section.class)}
				{@const completed = completedForSection(section.class)}

				<div class="mb-4">
					<!-- Section divider with name and counter -->
					<div class="mb-2 flex items-center gap-2 px-2 md:px-3">
						<div class="h-px flex-1 bg-border/40"></div>
						<span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/50">
							{section.name}
						</span>
						<span class="rounded-full bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground/60">
							{section.root_count}/{section.max_tasks}
						</span>
						{#if section.can_add}
							<LockOpenIcon class="h-3 w-3 {sectionColor(section.class)}" />
						{:else}
							<LockIcon class="h-3 w-3 text-muted-foreground/40" />
						{/if}
						<div class="h-px flex-1 bg-border/40"></div>
					</div>

					<!-- Active task list -->
					{#if section.tasks.length > 0}
						<div class="space-y-px px-1">
							{#each section.tasks as task (task.id)}
								{@render taskRow(task, 'text-sm')}
							{/each}
						</div>
					{/if}

					<!-- Add task input -->
					{#if section.can_add}
						<div class="mt-1 flex items-center gap-2 px-3 md:px-4">
							<PlusIcon class="h-3.5 w-3.5 shrink-0 text-muted-foreground/40" />
							<input
								type="text"
								class="flex-1 bg-transparent text-[13px] text-foreground/90 placeholder:text-muted-foreground/40 outline-none"
								placeholder={$t('troiki.addTask')}
								bind:value={addInputs[section.class]}
								onkeydown={(e) => handleKeydown(e, section.class)}
								disabled={submitting[section.class]}
							/>
						</div>
					{:else if !section.can_add && section.root_count < section.max_tasks}
						<div class="mt-1 px-3 md:px-4">
							<p class="text-[12px] text-muted-foreground/40">
								{$t('troiki.unlockHint', { values: { section: previousSectionName(section.class) } })}
							</p>
						</div>
					{/if}

					<!-- Completed tasks (collapsed by default) -->
					{#if completed.length > 0}
						<div class="mt-2 px-1">
							<button
								class="flex cursor-pointer items-center gap-1.5 px-2 py-2.5 text-xs text-muted-foreground/50 hover:text-muted-foreground/70 transition-colors"
								onclick={() => { completedExpanded[section.class] = !completedExpanded[section.class]; }}
							>
								{#if completedExpanded[section.class]}
									<ChevronDownIcon class="h-3.5 w-3.5" />
								{:else}
									<ChevronRightIcon class="h-3.5 w-3.5" />
								{/if}
								{$t('troiki.completed', { values: { count: completed.length } })}
							</button>
							{#if completedExpanded[section.class]}
								<div class="mt-1 space-y-px opacity-50">
									{#each completed as task (task.id)}
										<TaskItem {task} textSize="text-sm" completed={true} />
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}

			{#if troikiStore.sections.length === 0}
				<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
					<Layers3Icon class="mb-3 h-10 w-10 animate-float opacity-20" />
					<p class="text-sm">{$t('troiki.empty')}</p>
				</div>
			{/if}
		</div>
	{/if}
</div>
