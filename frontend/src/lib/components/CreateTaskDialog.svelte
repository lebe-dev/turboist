<script lang="ts">
	import { createTask, getLabels } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import type { DayPart, Label } from '$lib/api/types';
	import { onMount } from 'svelte';
	import TagIcon from '@lucide/svelte/icons/tag';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import XIcon from '@lucide/svelte/icons/x';
	import CheckIcon from '@lucide/svelte/icons/check';

	let { open = $bindable(false) }: { open: boolean } = $props();

	let content = $state('');
	let description = $state('');
	let selectedLabels = $state<string[]>([]);
	let priority = $state(1);
	let submitting = $state(false);

	let allLabels = $state<Label[]>([]);
	let showLabelPicker = $state(false);
	let showPriorityPicker = $state(false);
	let labelSearch = $state('');

	let contentInput: HTMLInputElement | undefined = $state();
	let dialogEl: HTMLDivElement | undefined = $state();

	const contextLabels = $derived.by(() => {
		const ctxId = contextsStore.activeContextId;
		if (!ctxId) return [];
		const ctx = contextsStore.contexts.find((c) => c.id === ctxId);
		return ctx?.filters.labels ?? [];
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

	onMount(async () => {
		try {
			allLabels = await getLabels();
		} catch {
			// ignore
		}
	});

	function getHourInTimezone(tz: string | undefined): number {
		if (!tz) return new Date().getHours();
		return parseInt(new Intl.DateTimeFormat('en-US', { timeZone: tz, hour: 'numeric', hour12: false }).format(new Date()));
	}

	function currentDayPartLabel(): string | null {
		const dayParts = tasksStore.config?.day_parts;
		if (!dayParts?.length || contextsStore.activeView !== 'today') return null;
		const hour = getHourInTimezone(tasksStore.config?.timezone);
		const match = dayParts.find((dp: DayPart) => hour >= dp.start && hour < dp.end);
		return match?.label ?? null;
	}

	$effect(() => {
		if (open) {
			const initial = [...contextLabels];
			const dpLabel = currentDayPartLabel();
			if (dpLabel && !initial.includes(dpLabel)) {
				initial.push(dpLabel);
			}
			selectedLabels = initial;
			content = '';
			description = '';
			priority = 1;
			showLabelPicker = false;
			showPriorityPicker = false;
			labelSearch = '';
			requestAnimationFrame(() => contentInput?.focus());
		}
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

	async function handleSubmit() {
		if (!content.trim() || submitting) return;
		submitting = true;
		try {
			await createTask(
				{
					content: content.trim(),
					description: description.trim(),
					labels: selectedLabels,
					priority
				},
				contextsStore.activeContextId ?? undefined
			);
			tasksStore.refresh();
			open = false;
		} catch (e) {
			console.error('Failed to create task', e);
		} finally {
			submitting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (showLabelPicker) {
				showLabelPicker = false;
			} else if (showPriorityPicker) {
				showPriorityPicker = false;
			} else {
				open = false;
			}
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === dialogEl) {
			open = false;
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={dialogEl}
		class="fixed inset-0 z-50 flex items-start justify-center pt-[15vh] bg-black/60 backdrop-blur-sm"
		onkeydown={handleKeydown}
		onclick={handleBackdropClick}
	>
		<div class="w-full max-w-lg mx-4 rounded-xl border border-border bg-popover shadow-2xl animate-fade-in-up">
			<!-- Content -->
			<div class="px-4 pt-4 pb-2">
				<input
					bind:this={contentInput}
					bind:value={content}
					type="text"
					placeholder="Task name"
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
					placeholder="Description"
					rows="1"
					class="mt-1 w-full resize-none bg-transparent text-sm text-muted-foreground placeholder:text-muted-foreground/30 focus:outline-none"
					oninput={(e) => {
						const target = e.currentTarget;
						target.style.height = 'auto';
						target.style.height = target.scrollHeight + 'px';
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
						Labels
					</button>

					{#if showLabelPicker}
						<div class="absolute top-full left-0 z-10 mt-1 w-56 rounded-lg border border-border bg-popover shadow-xl">
							<div class="p-2">
								<input
									bind:value={labelSearch}
									type="text"
									placeholder="Search labels..."
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
									<p class="px-2.5 py-2 text-[12px] text-muted-foreground">No labels found</p>
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
						{priority > 1 ? activePriority?.label : 'Priority'}
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
					onclick={() => (open = false)}
				>
					Cancel
				</button>
				<button
					class="rounded-lg px-4 py-1.5 text-[13px] font-medium transition-colors
						{content.trim()
							? 'bg-primary text-primary-foreground hover:bg-primary/90'
							: 'bg-muted text-muted-foreground cursor-not-allowed'}"
					disabled={!content.trim() || submitting}
					onclick={handleSubmit}
				>
					{submitting ? 'Adding...' : 'Add task'}
				</button>
			</div>
		</div>
	</div>
{/if}
