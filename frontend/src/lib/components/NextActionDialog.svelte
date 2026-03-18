<script lang="ts">
	import { createTask, getTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { nextActionStore } from '$lib/stores/next-action.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { logger } from '$lib/stores/logger';
	import { toast } from 'svelte-sonner';
	import type { Label, Task } from '$lib/api/types';
	import TagIcon from '@lucide/svelte/icons/tag';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import XIcon from '@lucide/svelte/icons/x';
	import CheckIcon from '@lucide/svelte/icons/check';
	import { t } from 'svelte-intl-precompile';

	let content = $state('');
	let description = $state('');
	let selectedLabels = $state<string[]>([]);
	let priority = $state(1);
	let submitting = $state(false);

	const allLabels = $derived(appStore.labels);
	let showLabelPicker = $state(false);
	let showPriorityPicker = $state(false);
	let labelSearch = $state('');

	let contentInput: HTMLInputElement | undefined = $state();
	let dialogEl: HTMLDivElement | undefined = $state();

	let parentChildren = $state<Task[]>([]);

	const open = $derived(nextActionStore.pendingAction !== null);
	const pending = $derived(nextActionStore.pendingAction);

	const contextLabels = $derived.by(() => {
		const ctxId = contextsStore.activeContextId;
		if (!ctxId) return [];
		const ctx = contextsStore.contexts.find((c) => c.id === ctxId);
		if (!ctx?.inherit_labels) return [];
		return ctx.filters.labels ?? [];
	});

	const filteredLabels = $derived.by(() => {
		if (!labelSearch) return allLabels;
		const q = labelSearch.toLowerCase();
		return allLabels.filter((l) => l.name.toLowerCase().includes(q));
	});

	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500' },
		{ value: 3, label: 'P2', color: 'text-amber-500' },
		{ value: 2, label: 'P3', color: 'text-blue-400' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground' }
	];

	const activePriority = $derived(priorityItems.find((p) => p.value === priority));

	function extractPrefix(content: string): string {
		const match = content.match(/^(.+?(?::\s|\s-\s))/);
		return match ? match[1] : '';
	}

	function detectPrefixFromSiblings(children: Task[]): string {
		const prefixes: Record<string, number> = {};
		for (const child of children) {
			const p = extractPrefix(child.content);
			if (p) prefixes[p] = (prefixes[p] ?? 0) + 1;
		}
		let best = '';
		let bestCount = 0;
		for (const [p, count] of Object.entries(prefixes)) {
			if (count > bestCount) {
				best = p;
				bestCount = count;
			}
		}
		return best;
	}

	// Find task by ID recursively in the tree
	function findTask(tasks: Task[], id: string): Task | null {
		for (const t of tasks) {
			if (t.id === id) return t;
			const found = findTask(t.children, id);
			if (found) return found;
		}
		return null;
	}

	function computePrefix(children: Task[], parentContent: string): string {
		return detectPrefixFromSiblings(children) || extractPrefix(parentContent);
	}

	// Reset form when dialog opens
	$effect(() => {
		if (!pending) return;

		// Set labels from completed task + context labels (respecting inherit_labels)
		const shouldInherit = (() => {
			const ctxId = contextsStore.activeContextId;
			if (!ctxId) return true;
			const ctx = contextsStore.contexts.find((c) => c.id === ctxId);
			return ctx?.inherit_labels ?? true;
		})();
		const labels = shouldInherit
			? [...new Set([...pending.completedTaskLabels, ...contextLabels])]
			: [];
		selectedLabels = labels;
		description = '';
		priority = 1;
		showLabelPicker = false;
		showPriorityPicker = false;
		labelSearch = '';

		// Load parent children for prefix detection (only for subtask next-actions)
		if (pending.parentId) {
			const parentInStore = findTask(tasksStore.tasks, pending.parentId);
			if (parentInStore) {
				parentChildren = parentInStore.children;
				content = computePrefix(parentInStore.children, pending.parentContent);
			} else {
				parentChildren = [];
				content = extractPrefix(pending.parentContent);
				getTask(pending.parentId)
					.then((t) => {
						parentChildren = t.children;
						const currentPrefix = extractPrefix(pending!.parentContent);
						if (content === currentPrefix || content === '') {
							content = computePrefix(t.children, pending!.parentContent);
						}
					})
					.catch(() => {});
			}
		} else {
			// Standalone follow-up — no prefix detection
			parentChildren = [];
			content = '';
		}

		requestAnimationFrame(() => contentInput?.focus());
	});

	function toggleLabel(name: string) {
		if (selectedLabels.includes(name)) {
			selectedLabels = selectedLabels.filter((l) => l !== name);
		} else {
			selectedLabels = [...selectedLabels, name];
		}
	}

	function isContextLabel(name: string): boolean {
		return contextLabels.includes(name);
	}

	function handleSkip() {
		nextActionStore.dismiss();
	}

	function handleSubmit() {
		if (!content.trim() || submitting || !pending) return;
		const trimmedContent = content.trim();
		const trimmedDesc = description.trim();
		const labels = [...selectedLabels];
		const pri = priority;
		const parentId = pending.parentId;
		const context = contextsStore.activeContextId ?? undefined;

		// Optimistic: close immediately and add temp task
		const tempId = `temp-${Date.now()}`;
		const optimistic: Task = {
			id: tempId,
			content: trimmedContent,
			description: trimmedDesc,
			project_id: '',
			section_id: null,
			parent_id: parentId,
			labels,
			priority: pri,
			due: null,
			sub_task_count: 0,
			completed_sub_task_count: 0,
			completed_at: null,
			added_at: new Date().toISOString(),
			is_project_task: false,
			children: []
		};
		nextActionStore.dismiss();

		if (parentId) {
			// Add as child of parent task in local store
			tasksStore.updateTaskLocal(parentId, (t) => ({
				...t,
				children: [...t.children, optimistic],
				sub_task_count: t.sub_task_count + 1
			}));
		} else {
			// Standalone follow-up — add to top of task list
			tasksStore.addTaskLocal(optimistic);
		}

		createTask(
			{ content: trimmedContent, description: trimmedDesc, labels, priority: pri, ...(parentId ? { parent_id: parentId } : {}) },
			context
		).catch((e) => {
			logger.error('tasks', `create next action failed: ${e}`);
			toast.error($t('errors.createFailed'));
			tasksStore.refresh();
		});
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (showLabelPicker) {
				showLabelPicker = false;
			} else if (showPriorityPicker) {
				showPriorityPicker = false;
			} else {
				handleSkip();
			}
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === dialogEl) {
			handleSkip();
		}
	}
</script>

{#if open && pending}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={dialogEl}
		class="fixed inset-0 z-50 flex items-start justify-center pt-[15vh] bg-black/60 backdrop-blur-sm"
		onkeydown={handleKeydown}
		onclick={handleBackdropClick}
	>
		<div class="w-full max-w-lg mx-4 rounded-xl border border-border bg-popover shadow-2xl animate-fade-in-up">
			<!-- Header -->
			<div class="px-4 pt-3 pb-1">
				<p class="text-[12px] font-medium text-muted-foreground">
					{#if pending.parentId}
						{$t('task.nextAction')} <span class="text-foreground">{pending.parentContent}</span>
					{:else}
						{$t('task.followUp')} <span class="text-foreground">{pending.completedTaskContent}</span>
					{/if}
				</p>
			</div>

			<!-- Content -->
			<div class="px-4 pt-2 pb-2">
				<input
					bind:this={contentInput}
					bind:value={content}
					type="text"
					placeholder={$t('task.nextActionPlaceholder')}
					class="w-full bg-transparent text-lg font-medium text-foreground placeholder:text-muted-foreground/40 focus:outline-none"
					onkeydown={(e) => {
						if (e.key === 'Enter' && !e.shiftKey) {
							e.preventDefault();
							handleSubmit();
						}
					}}
				/>
				<textarea
					bind:value={description}
					placeholder={$t('task.description')}
					rows="1"
					class="mt-1 w-full resize-none bg-transparent text-sm text-muted-foreground placeholder:text-muted-foreground/30 focus:outline-none"
					oninput={(e) => {
						const target = e.currentTarget;
						target.style.height = 'auto';
						target.style.height = target.scrollHeight + 'px';
					}}
					onkeydown={(e) => {
						if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
							e.preventDefault();
							handleSubmit();
						}
					}}
				></textarea>
			</div>

			<!-- Selected labels -->
			{#if selectedLabels.length > 0}
				<div class="flex flex-wrap gap-1.5 px-4 pb-2">
					{#each selectedLabels as label (label)}
						<button
							class="flex items-center gap-1 rounded-md px-2 py-0.5 text-[12px] font-medium transition-colors
								{isContextLabel(label)
									? 'bg-primary/10 text-primary'
									: 'bg-muted text-muted-foreground hover:bg-muted/80'}"
							onclick={() => toggleLabel(label)}
						>
							{label}
							{#if !isContextLabel(label)}
								<XIcon class="h-3 w-3" />
							{/if}
						</button>
					{/each}
				</div>
			{/if}

			<!-- Toolbar -->
			<div class="flex items-center gap-1 px-3 pb-3">
				<div class="relative">
					<button
						class="flex items-center gap-1.5 rounded-md border border-border/50 px-2.5 py-1.5 text-[12px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						onclick={() => { showLabelPicker = !showLabelPicker; showPriorityPicker = false; }}
					>
						<TagIcon class="h-3.5 w-3.5" />
						{$t('task.labels')}
					</button>

					{#if showLabelPicker}
						<div class="absolute top-full left-0 z-10 mt-1 w-56 rounded-lg border border-border bg-popover shadow-xl">
							<div class="p-2">
								<input
									bind:value={labelSearch}
									type="text"
									placeholder={$t('task.searchLabels')}
									class="w-full rounded-md border border-border/50 bg-transparent px-2.5 py-1.5 text-[12px] text-foreground placeholder:text-muted-foreground/40 focus:border-border focus:outline-none"
								/>
							</div>
							<div class="max-h-48 overflow-y-auto px-1 pb-1">
								{#each filteredLabels as label (label.id)}
									<button
										class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12px] text-foreground transition-colors hover:bg-accent"
										onclick={() => toggleLabel(label.name)}
									>
										<div class="flex h-4 w-4 items-center justify-center rounded border border-border/50
											{selectedLabels.includes(label.name) ? 'bg-primary border-primary' : ''}">
											{#if selectedLabels.includes(label.name)}
												<CheckIcon class="h-3 w-3 text-primary-foreground" strokeWidth={3} />
											{/if}
										</div>
										{label.name}
									</button>
								{/each}
								{#if filteredLabels.length === 0}
									<p class="px-2.5 py-2 text-[12px] text-muted-foreground">{$t('task.noLabelsFound')}</p>
								{/if}
							</div>
						</div>
					{/if}
				</div>

				<div class="relative">
					<button
						class="flex items-center gap-1.5 rounded-md border border-border/50 px-2.5 py-1.5 text-[12px] transition-colors hover:bg-accent
							{priority > 1 ? activePriority?.color ?? 'text-muted-foreground' : 'text-muted-foreground hover:text-foreground'}"
						onclick={() => { showPriorityPicker = !showPriorityPicker; showLabelPicker = false; }}
					>
						<FlagIcon class="h-3.5 w-3.5" />
						{priority > 1 ? activePriority?.label : $t('task.priority')}
					</button>

					{#if showPriorityPicker}
						<div class="absolute top-full left-0 z-10 mt-1 w-36 rounded-lg border border-border bg-popover shadow-xl">
							<div class="px-1 py-1">
								{#each priorityItems as p (p.value)}
									<button
										class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12px] transition-colors hover:bg-accent
											{priority === p.value ? 'bg-accent' : ''}"
										onclick={() => { priority = p.value; showPriorityPicker = false; }}
									>
										<FlagIcon class="h-3.5 w-3.5 {p.color}" />
										<span class={p.color}>{p.label}</span>
									</button>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>

			<!-- Footer -->
			<div class="flex items-center justify-end gap-2 border-t border-border/50 px-4 py-3">
				<button
					class="rounded-lg px-4 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					onclick={handleSkip}
				>
					{$t('dialog.skip')}
				</button>
				<button
					class="rounded-lg px-4 py-1.5 text-[13px] font-medium transition-colors
						{content.trim()
							? 'bg-primary text-primary-foreground hover:bg-primary/90'
							: 'bg-muted text-muted-foreground cursor-not-allowed'}"
					disabled={!content.trim() || submitting}
					onclick={handleSubmit}
				>
					{submitting ? $t('task.adding') : $t('task.add')}
				</button>
			</div>
		</div>
	</div>
{/if}
