<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { tick } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import type { DayPart, Priority, TaskInput } from '$lib/api/types';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import PriorityPicker from './PriorityPicker.svelte';
	import DayPartPicker from './DayPartPicker.svelte';
	import RecurrencePicker from './RecurrencePicker.svelte';
	import { dayKeyInTz, dayStartUtcInTz, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { clickOutside } from '$lib/actions/clickOutside';
	import XIcon from 'phosphor-svelte/lib/X';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import SparkleIcon from 'phosphor-svelte/lib/Sparkle';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import CheckIcon from 'phosphor-svelte/lib/Check';

	let {
		open = $bindable(false),
		defaultProjectId = null,
		defaultLabelIds = [],
		defaultDueDate = '',
		emptyProjectLabel = 'Inbox',
		onSubmit
	}: {
		open?: boolean;
		defaultProjectId?: number | null;
		defaultLabelIds?: Array<string | number>;
		defaultDueDate?: string;
		emptyProjectLabel?: string;
		onSubmit?: (
			payload: TaskInput,
			target: { projectId: number | null; labels: string[] }
		) => void | Promise<void>;
	} = $props();

	let title = $state('');
	let description = $state('');
	let priority = $state<Priority>('no-priority');
	let dayPart = $state<DayPart>('none');
	// svelte-ignore state_referenced_locally
	let dueDate = $state<string>(defaultDueDate ?? '');
	// svelte-ignore state_referenced_locally
	let projectId = $state<string>(defaultProjectId ? String(defaultProjectId) : '');
	// svelte-ignore state_referenced_locally
	let labelIds = $state<string[]>(defaultLabelIds.map(String));
	let recurrenceRule = $state<string | null>(null);
	let submitting = $state(false);
	let labelMenuOpen = $state(false);
	let projectMenuOpen = $state(false);
	let projectQuery = $state('');
	let projectSearchInput = $state<HTMLInputElement | null>(null);
	let dismissedAutoLabels = $state<string[]>([]);

	const visibleProjects = $derived(
		projectsStore.items
			.filter((p) => p.status !== 'completed')
			.sort((a, b) => a.title.localeCompare(b.title))
	);
	const filteredProjects = $derived.by(() => {
		const q = projectQuery.trim().toLowerCase();
		if (!q) return visibleProjects;
		return visibleProjects.filter((p) => p.title.toLowerCase().includes(q));
	});
	const inboxMatchesQuery = $derived.by(() => {
		const q = projectQuery.trim().toLowerCase();
		if (!q) return true;
		return emptyProjectLabel.toLowerCase().includes(q);
	});

	async function openProjectMenu(): Promise<void> {
		projectMenuOpen = !projectMenuOpen;
		if (projectMenuOpen) {
			projectQuery = '';
			await tick();
			projectSearchInput?.focus();
		}
	}

	function selectProject(id: string): void {
		projectId = id;
		projectMenuOpen = false;
	}

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);
	const selectedLabels = $derived(
		labelIds
			.map((id) => allLabels.find((l) => String(l.id) === id))
			.filter((l): l is (typeof allLabels)[number] => !!l)
	);
	const autoLabelRules = $derived(configStore.value?.autoLabels ?? []);
	const detectedAutoLabels = $derived.by(() => {
		const matched: string[] = [];
		const explicitNames = selectedLabels.map((l) => l.name);
		const lowerTitle = title.toLowerCase();
		for (const rule of autoLabelRules) {
			if (!rule.mask) continue;
			const hay = rule.ignoreCase === false ? title : lowerTitle;
			const needle = rule.ignoreCase === false ? rule.mask : rule.mask.toLowerCase();
			if (!hay.includes(needle)) continue;
			if (matched.includes(rule.label)) continue;
			if (explicitNames.includes(rule.label)) continue;
			if (dismissedAutoLabels.includes(rule.label)) continue;
			matched.push(rule.label);
		}
		return matched;
	});

	function dismissAutoLabel(name: string): void {
		if (!dismissedAutoLabels.includes(name)) {
			dismissedAutoLabels = [...dismissedAutoLabels, name];
		}
	}
	const projectName = $derived(
		projectsStore.items.find((p) => String(p.id) === projectId)?.title ?? emptyProjectLabel
	);
	const todayKey = $derived(dayKeyInTz(new Date(), configStore.value?.timezone ?? null));
	const tomorrowKey = $derived(shiftDayKey(todayKey, 1));
	const isToday = $derived(dueDate === todayKey);
	const isTomorrow = $derived(dueDate === tomorrowKey);
	const isCustomDate = $derived(!!dueDate && !isToday && !isTomorrow);

	let dateInputEl: HTMLInputElement | undefined = $state();
	let titleEl: HTMLTextAreaElement | undefined = $state();
	let descriptionEl: HTMLTextAreaElement | undefined = $state();

	function setDate(value: string) {
		dueDate = dueDate === value ? '' : value;
	}

	function openDatePicker() {
		const el = dateInputEl;
		if (!el) return;
		try {
			if (typeof el.showPicker === 'function') el.showPicker();
			else el.click();
		} catch {
			el.click();
		}
	}

	function autoGrow(el: HTMLTextAreaElement | undefined) {
		if (!el) return;
		el.style.height = 'auto';
		el.style.height = `${el.scrollHeight}px`;
	}

	$effect(() => {
		void description;
		autoGrow(descriptionEl);
	});

	$effect(() => {
		void title;
		autoGrow(titleEl);
	});

	$effect(() => {
		if (open) {
			queueMicrotask(() => {
				autoGrow(titleEl);
				autoGrow(descriptionEl);
			});
		}
	});

	function onTitleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
			e.preventDefault();
			void submit(e);
		}
	}

	function reset() {
		title = '';
		description = '';
		priority = 'no-priority';
		dayPart = 'none';
		dueDate = defaultDueDate ?? '';
		recurrenceRule = null;
		projectId = defaultProjectId ? String(defaultProjectId) : '';
		labelIds = defaultLabelIds.map(String);
		labelMenuOpen = false;
		dismissedAutoLabels = [];
	}

	let prevOpen = false;
	$effect(() => {
		if (open && !prevOpen) {
			dueDate = defaultDueDate ?? '';
			projectId = defaultProjectId ? String(defaultProjectId) : '';
			labelIds = defaultLabelIds.map(String);
		}
		prevOpen = open;
	});

	function toggleLabel(id: string) {
		labelIds = labelIds.includes(id)
			? labelIds.filter((x) => x !== id)
			: [...labelIds, id];
	}

	async function submit(e: Event) {
		e.preventDefault();
		if (!title.trim() || submitting) return;
		submitting = true;
		try {
			const payload: TaskInput = {
				title: title.trim(),
				description: description.trim() || undefined,
				priority,
				dayPart,
				dueAt: dueDate
					? toIsoUtc(dayStartUtcInTz(dueDate, configStore.value?.timezone ?? null))
					: null,
				dueHasTime: false,
				recurrenceRule,
				labels: labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n),
				removedAutoLabels: dismissedAutoLabels.length > 0 ? [...dismissedAutoLabels] : undefined
			};
			const target = {
				projectId: projectId ? Number(projectId) : null,
				labels: payload.labels ?? []
			};
			await onSubmit?.(payload, target);
			reset();
			open = false;
		} finally {
			submitting = false;
		}
	}

	function onOpenChange(value: boolean) {
		open = value;
		if (!value) reset();
	}
</script>

<DialogPrimitive.Root bind:open onOpenChange={onOpenChange}>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
		/>
		<DialogPrimitive.Content
			class="fixed left-1/2 top-[15%] z-50 w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 rounded-xl border border-border bg-popover text-popover-foreground shadow-xl outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
		>
			<DialogPrimitive.Title class="sr-only">Quick add task</DialogPrimitive.Title>
			<DialogPrimitive.Description class="sr-only">
				Title plus optional description, project, priority, due date, day part, and labels.
			</DialogPrimitive.Description>

			<form onsubmit={submit} class="flex flex-col">
				<div class="px-5 pt-5 pb-3">
					<!-- svelte-ignore a11y_autofocus -->
					<textarea
						bind:this={titleEl}
						bind:value={title}
						placeholder="Task name"
						aria-label="Task name"
						rows="1"
						oninput={(e) => autoGrow(e.currentTarget as HTMLTextAreaElement)}
						onkeydown={onTitleKeydown}
						class="block w-full resize-none overflow-hidden break-words bg-transparent text-lg font-medium leading-tight outline-none placeholder:text-muted-foreground/70"
						autofocus
					></textarea>
					<textarea
						bind:this={descriptionEl}
						bind:value={description}
						placeholder="Description"
						aria-label="Description"
						rows="1"
						oninput={(e) => autoGrow(e.currentTarget as HTMLTextAreaElement)}
						class="mt-2 block w-full resize-none overflow-hidden bg-transparent text-sm leading-relaxed text-foreground outline-none placeholder:text-muted-foreground/60"
					></textarea>

					{#if selectedLabels.length > 0 || detectedAutoLabels.length > 0}
						<div class="mt-3 flex flex-wrap items-center gap-1.5">
							{#each selectedLabels as label (label.id)}
								<button
									type="button"
									onclick={() => toggleLabel(String(label.id))}
									class="group/chip inline-flex items-center gap-1 rounded-full bg-accent px-2 py-0.5 text-xs font-medium text-accent-foreground transition-colors hover:bg-accent/70"
								>
									{#if label.color}
										<span
											class="size-1.5 rounded-full"
											style={`background-color: ${label.color}`}
										></span>
									{/if}
									<span>{label.name}</span>
									<XIcon class="size-3 opacity-60 transition-opacity group-hover/chip:opacity-100" />
								</button>
							{/each}
							{#each detectedAutoLabels as name (name)}
								<span
									class="group/auto inline-flex items-center gap-1 rounded-full border border-dashed border-primary/40 bg-primary/5 px-2 py-0.5 text-xs font-medium text-primary"
									title="Auto-detected from title — will be applied on save"
								>
									<SparkleIcon class="size-3" weight="fill" />
									<span>{name}</span>
									<button
										type="button"
										onclick={() => dismissAutoLabel(name)}
										aria-label={`Reject auto-label ${name}`}
										class="opacity-60 transition-opacity hover:opacity-100"
									>
										<XIcon class="size-3" />
									</button>
								</span>
							{/each}
						</div>
					{/if}

					<div class="mt-4 flex flex-wrap items-center gap-2">
						<div
							class="inline-flex w-fit items-center gap-0.5 rounded-md border border-border bg-background p-0.5"
							role="group"
							aria-label="Due date"
						>
							<button
								type="button"
								onclick={() => setDate(todayKey)}
								aria-pressed={isToday}
								class="inline-flex h-7 items-center rounded-[5px] px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
								class:bg-accent={isToday}
								class:text-foreground={isToday}
								class:text-muted-foreground={!isToday}
								class:hover:bg-accent={!isToday}
								class:hover:text-foreground={!isToday}
							>
								Today
							</button>
							<button
								type="button"
								onclick={() => setDate(tomorrowKey)}
								aria-pressed={isTomorrow}
								class="inline-flex h-7 items-center rounded-[5px] px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
								class:bg-accent={isTomorrow}
								class:text-foreground={isTomorrow}
								class:text-muted-foreground={!isTomorrow}
								class:hover:bg-accent={!isTomorrow}
								class:hover:text-foreground={!isTomorrow}
							>
								Tomorrow
							</button>
							<div
								class="relative inline-flex h-7 items-center gap-1 rounded-[5px] px-2 text-xs font-medium transition-colors focus-within:ring-[2px] focus-within:ring-ring/50"
								class:bg-accent={isCustomDate}
								class:text-foreground={isCustomDate}
								class:text-muted-foreground={!isCustomDate}
								class:hover:bg-accent={!isCustomDate}
								class:hover:text-foreground={!isCustomDate}
							>
								{#if isCustomDate}
									<span class="font-mono text-[11px]">{dueDate}</span>
								{:else}
									<DotsThreeIcon class="size-4" weight="bold" />
								{/if}
								<input
									bind:this={dateInputEl}
									bind:value={dueDate}
									type="date"
									aria-label="Custom date"
									title={isCustomDate ? dueDate : 'Pick a date'}
									onclick={(e) => {
										e.stopPropagation();
										openDatePicker();
									}}
									class="absolute inset-0 size-full cursor-pointer opacity-0"
								/>
							</div>
						</div>

						<PriorityPicker bind:value={priority} />

						<DayPartPicker bind:value={dayPart} />

						<RecurrencePicker bind:value={recurrenceRule} />

						{#if allLabels.length > 0}
							<div
								class="relative"
								use:clickOutside={() => (labelMenuOpen = false)}
							>
								<button
									type="button"
									onclick={() => (labelMenuOpen = !labelMenuOpen)}
									aria-expanded={labelMenuOpen}
									class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground aria-expanded:bg-accent"
								>
									<TagIcon class="size-3.5" />
									<span>Labels</span>
								</button>
								{#if labelMenuOpen}
									<div
										class="absolute left-0 top-9 z-10 flex max-h-64 w-56 flex-col gap-1 overflow-y-auto rounded-md border border-border bg-popover p-2 shadow-lg"
										role="menu"
									>
										{#each allLabels as label (label.id)}
											{@const id = String(label.id)}
											{@const active = labelIds.includes(id)}
											<button
												type="button"
												onclick={() => toggleLabel(id)}
												class="inline-flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors"
												class:bg-accent={active}
												class:text-accent-foreground={active}
												class:hover:bg-accent={!active}
											>
												{#if label.color}
													<span
														class="size-2 rounded-full"
														style={`background-color: ${label.color}`}
													></span>
												{/if}
												<span class="flex-1 truncate">{label.name}</span>
												{#if active}
													<XIcon class="size-3 opacity-60" />
												{/if}
											</button>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</div>
				</div>

				<div
					class="flex items-center justify-between gap-3 border-t border-border bg-muted/30 px-5 py-3"
				>
					<div class="relative" use:clickOutside={() => (projectMenuOpen = false)}>
						<button
							type="button"
							onclick={openProjectMenu}
							aria-expanded={projectMenuOpen}
							class="inline-flex h-8 max-w-[14rem] items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground aria-expanded:bg-accent"
						>
							<span class="truncate">{projectName}</span>
						</button>
						{#if projectMenuOpen}
							<div
								class="absolute bottom-9 left-0 z-10 flex w-64 flex-col rounded-md border border-border bg-popover shadow-lg"
								role="menu"
							>
								<div class="flex items-center gap-2 border-b border-border px-2.5 py-1.5">
									<MagnifyingGlassIcon class="size-3.5 text-muted-foreground" />
									<input
										bind:this={projectSearchInput}
										bind:value={projectQuery}
										type="text"
										placeholder="Search projects..."
										class="h-6 w-full bg-transparent text-xs outline-none placeholder:text-muted-foreground"
										onkeydown={(e) => {
											if (e.key === 'Escape') {
												e.stopPropagation();
												projectMenuOpen = false;
											}
										}}
									/>
								</div>
								<div class="flex max-h-56 flex-col gap-0.5 overflow-y-auto p-1">
									{#if inboxMatchesQuery}
										{@const active = projectId === ''}
										<button
											type="button"
											onclick={() => selectProject('')}
											class="inline-flex items-center gap-2 rounded px-2 py-1.5 text-left text-xs transition-colors"
											class:bg-accent={active}
											class:text-accent-foreground={active}
											class:hover:bg-accent={!active}
										>
											<span class="flex-1 truncate">{emptyProjectLabel}</span>
											{#if active}
												<CheckIcon class="size-3.5 opacity-70" />
											{/if}
										</button>
									{/if}
									{#each filteredProjects as project (project.id)}
										{@const id = String(project.id)}
										{@const active = projectId === id}
										<button
											type="button"
											onclick={() => selectProject(id)}
											class="inline-flex items-center gap-2 rounded px-2 py-1.5 text-left text-xs transition-colors"
											class:bg-accent={active}
											class:text-accent-foreground={active}
											class:hover:bg-accent={!active}
										>
											<span class="flex-1 truncate">{project.title}</span>
											{#if active}
												<CheckIcon class="size-3.5 opacity-70" />
											{/if}
										</button>
									{/each}
									{#if !inboxMatchesQuery && filteredProjects.length === 0}
										<div class="px-2 py-3 text-center text-xs text-muted-foreground">
											No matches
										</div>
									{/if}
								</div>
							</div>
						{/if}
					</div>

					<div class="flex items-center gap-2">
						<DialogPrimitive.Close>
							{#snippet child({ props })}
								<Button {...props} variant="ghost" size="sm" type="button">Cancel</Button>
							{/snippet}
						</DialogPrimitive.Close>
						<Button type="submit" size="sm" disabled={!title.trim() || submitting}>
							Add task
						</Button>
					</div>
				</div>
			</form>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
