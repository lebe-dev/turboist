<script lang="ts">
	import { Dialog as DialogPrimitive, Popover as PopoverPrimitive } from 'bits-ui';
	import { tick } from 'svelte';
	import { parseDate, type DateValue } from '@internationalized/date';
	import { Button } from '$lib/components/ui/button';
	import { Calendar } from '$lib/components/ui/calendar';
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
		defaultPriority = 'no-priority',
		defaultDayPart = 'none',
		defaultParentId = null,
		defaultSectionId = null,
		emptyProjectLabel = 'Inbox',
		onSubmit
	}: {
		open?: boolean;
		defaultProjectId?: number | null;
		defaultLabelIds?: Array<string | number>;
		defaultDueDate?: string;
		defaultPriority?: Priority;
		defaultDayPart?: DayPart;
		defaultParentId?: number | null;
		defaultSectionId?: number | null;
		emptyProjectLabel?: string;
		onSubmit?: (
			payload: TaskInput,
			target: {
				projectId: number | null;
				labels: string[];
				parentId: number | null;
				sectionId: number | null;
			}
		) => void | Promise<void>;
	} = $props();

	let titles = $state('');
	let description = $state('');
	// svelte-ignore state_referenced_locally
	let priority = $state<Priority>(defaultPriority);
	// svelte-ignore state_referenced_locally
	let dayPart = $state<DayPart>(defaultDayPart);
	// svelte-ignore state_referenced_locally
	let dueDate = $state<string>(defaultDueDate ?? '');
	// svelte-ignore state_referenced_locally
	let projectId = $state<string>(defaultProjectId ? String(defaultProjectId) : '');
	// svelte-ignore state_referenced_locally
	let labelIds = $state<string[]>(defaultLabelIds.map(String));
	// svelte-ignore state_referenced_locally
	let parentId = $state<number | null>(defaultParentId);
	// svelte-ignore state_referenced_locally
	let sectionId = $state<number | null>(defaultSectionId);
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
	const titleLines = $derived(titles.split('\n').map((l) => l.trim()).filter(Boolean));
	const isMultiTask = $derived(titleLines.length > 1);

	const autoLabelRules = $derived(configStore.value?.autoLabels ?? []);
	const detectedAutoLabels = $derived.by(() => {
		const matched: string[] = [];
		const explicitNames = selectedLabels.map((l) => l.name);
		const lowerTitles = titles.toLowerCase();
		for (const rule of autoLabelRules) {
			if (!rule.mask) continue;
			const hay = rule.ignoreCase === false ? titles : lowerTitles;
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

	let datePopoverOpen = $state(false);
	let titlesEl: HTMLTextAreaElement | undefined = $state();
	let descriptionEl: HTMLTextAreaElement | undefined = $state();

	const calendarValue = $derived<DateValue | undefined>(
		dueDate ? parseDate(dueDate) : undefined
	);

	function pad(n: number): string {
		return n < 10 ? `0${n}` : String(n);
	}

	function setDate(value: string) {
		dueDate = dueDate === value ? '' : value;
	}

	function setCalendarValue(v: DateValue | undefined): void {
		if (!v) {
			dueDate = '';
		} else {
			dueDate = `${v.year}-${pad(v.month)}-${pad(v.day)}`;
		}
		datePopoverOpen = false;
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
		void titles;
		autoGrow(titlesEl);
	});

	$effect(() => {
		if (open) {
			queueMicrotask(() => {
				autoGrow(titlesEl);
				autoGrow(descriptionEl);
			});
		}
	});

	function reset() {
		titles = '';
		description = '';
		priority = defaultPriority;
		dayPart = defaultDayPart;
		dueDate = defaultDueDate ?? '';
		recurrenceRule = null;
		projectId = defaultProjectId ? String(defaultProjectId) : '';
		labelIds = defaultLabelIds.map(String);
		parentId = defaultParentId;
		sectionId = defaultSectionId;
		labelMenuOpen = false;
		dismissedAutoLabels = [];
	}

	let prevOpen = false;
	$effect(() => {
		if (open && !prevOpen) {
			dueDate = defaultDueDate ?? '';
			projectId = defaultProjectId ? String(defaultProjectId) : '';
			labelIds = defaultLabelIds.map(String);
			priority = defaultPriority;
			dayPart = defaultDayPart;
			parentId = defaultParentId;
			sectionId = defaultSectionId;
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
		if (titleLines.length === 0 || submitting) return;
		submitting = true;
		try {
			const resolvedLabels = labelIds
				.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
				.filter((n): n is string => !!n);
			const commonPayload = {
				description: description.trim() || undefined,
				priority,
				dayPart,
				dueAt: dueDate
					? toIsoUtc(dayStartUtcInTz(dueDate, configStore.value?.timezone ?? null))
					: null,
				dueHasTime: false as const,
				recurrenceRule,
				labels: resolvedLabels,
				removedAutoLabels: dismissedAutoLabels.length > 0 ? [...dismissedAutoLabels] : undefined
			};
			const target = {
				projectId: projectId ? Number(projectId) : null,
				labels: resolvedLabels,
				parentId,
				sectionId
			};
			for (const line of titleLines) {
				const payload: TaskInput = { ...commonPayload, title: line };
				await onSubmit?.(payload, target);
			}
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
						bind:this={titlesEl}
						bind:value={titles}
						placeholder="Task name (one per line)"
						aria-label="Task names"
						rows="1"
						oninput={(e) => autoGrow(e.currentTarget as HTMLTextAreaElement)}
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
							<PopoverPrimitive.Root bind:open={datePopoverOpen}>
								<PopoverPrimitive.Trigger
									aria-pressed={isCustomDate}
									aria-label="Custom date"
									title={isCustomDate ? dueDate : 'Pick a date'}
									class="inline-flex h-7 items-center gap-1 rounded-[5px] px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50 {isCustomDate
										? 'bg-accent text-foreground'
										: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
								>
									{#if isCustomDate}
										<span class="font-mono text-[11px]">{dueDate}</span>
									{:else}
										<DotsThreeIcon class="size-4" weight="bold" />
									{/if}
								</PopoverPrimitive.Trigger>
								<PopoverPrimitive.Portal>
									<PopoverPrimitive.Content
										align="start"
										sideOffset={6}
										class="z-[60] rounded-md border border-border bg-popover text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
									>
										<Calendar
											type="single"
											value={calendarValue}
											onValueChange={setCalendarValue}
											captionLayout="dropdown"
										/>
									</PopoverPrimitive.Content>
								</PopoverPrimitive.Portal>
							</PopoverPrimitive.Root>
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
						<Button type="submit" size="sm" disabled={titleLines.length === 0 || submitting}>
							{isMultiTask ? `Add ${titleLines.length} tasks` : 'Add task'}
						</Button>
					</div>
				</div>
			</form>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
