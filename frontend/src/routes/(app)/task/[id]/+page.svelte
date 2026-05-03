<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import XIcon from 'phosphor-svelte/lib/X';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import TextAlignStartIcon from 'phosphor-svelte/lib/TextAlignLeft';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { Button } from '$lib/components/ui/button';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { configStore } from '$lib/stores/config.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import PriorityPicker from '$lib/components/task/PriorityPicker.svelte';
	import DayPartPicker from '$lib/components/task/DayPartPicker.svelte';
	import RecurrencePicker from '$lib/components/task/RecurrencePicker.svelte';
	import TaskActionsMenu from '$lib/components/task/TaskActionsMenu.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import { dayKeyInTz, dayStartUtcInTz, parseIso, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { describeError, toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const taskId = $derived(Number(page.params.id));

	let task = $state<Task | null>(null);
	let notFound = $state(false);
	let saving = $state(false);

	const subtasks = useListMutator<Task>();
	let newSubtaskTitle = $state('');
	let creatingSubtask = $state(false);
	let subtaskInputEl = $state<HTMLInputElement | undefined>();

	let title = $state('');
	let description = $state('');
	let descriptionFocused = $state(false);
	let priority = $state<Priority>('no-priority');
	let dayPart = $state<DayPart>('none');
	let dueDate = $state('');
	let recurrence = $state<string | null>(null);
	let labelIds = $state<string[]>([]);
	let removedAuto = $state<string[]>([]);

	let dateInputEl = $state<HTMLInputElement | undefined>();

	const todayKey = $derived(dayKeyInTz(new Date(), configStore.value?.timezone ?? null));
	const tomorrowKey = $derived(shiftDayKey(todayKey, 1));
	const isToday = $derived(dueDate === todayKey);
	const isTomorrow = $derived(dueDate === tomorrowKey);
	const isCustomDate = $derived(!!dueDate && !isToday && !isTomorrow);

	// Non-reactive flag — guards against auto-save during initial hydration
	let allowSave = false;
	let saveTimer: ReturnType<typeof setTimeout> | null = null;

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);
	const project = $derived(
		task?.projectId ? projectsStore.items.find((p) => p.id === task!.projectId) : null
	);
	const autoLabelNames = $derived(
		new Set((configStore.value?.autoLabels ?? []).map((r) => r.label))
	);
	const selectedLabels = $derived(
		labelIds
			.map((id) => allLabels.find((l) => String(l.id) === id))
			.filter((l): l is (typeof allLabels)[number] => !!l)
	);

	function hydrate(t: Task): void {
		allowSave = false;
		task = t;
		title = t.title;
		description = t.description ?? '';
		priority = t.priority;
		dayPart = t.dayPart;
		recurrence = t.recurrenceRule ?? null;
		const dt = parseIso(t.dueAt);
		if (dt) {
			dueDate = dayKeyInTz(dt, configStore.value?.timezone ?? null);
		} else {
			dueDate = '';
		}
		labelIds = t.labels.map((l) => String(l.id));
		removedAuto = [];
		// Permit auto-save only after all reactive effects from hydration have flushed
		setTimeout(() => {
			allowSave = true;
		}, 0);
	}

	function scheduleSave(): void {
		if (!allowSave || !task || !title.trim()) return;
		if (saveTimer !== null) clearTimeout(saveTimer);
		saveTimer = setTimeout(() => void save(), 800);
	}

	// Watch picker bindings for auto-save
	$effect(() => {
		void priority;
		scheduleSave();
	});
	$effect(() => {
		void dayPart;
		scheduleSave();
	});
	$effect(() => {
		void recurrence;
		scheduleSave();
	});
	$effect(() => {
		if (task) viewFilterStore.setTitle(task.title);
	});

	const loader = usePageLoad(
		async (isValid) => {
			notFound = false;
			task = null;
			subtasks.items = [];
			if (!Number.isFinite(taskId)) {
				notFound = true;
				return;
			}
			const client = getApiClient();
			const [t, subs] = await Promise.all([
				tasksApi.get(client, taskId),
				tasksApi.listSubtasks(client, taskId).catch((err) => {
					if (err instanceof ApiError && err.code === 'not_found') return null;
					throw err;
				})
			]);
			if (!isValid()) return;
			hydrate(t);
			if (subs) subtasks.items = subs.items;
		},
		{
			autoLoad: false,
			initialLoading: true,
			onError(err) {
				if (err instanceof ApiError && err.code === 'not_found') {
					notFound = true;
					return;
				}
				toast.error(describeError(err, 'Failed to load task'));
			}
		}
	);

	const pageMutator: ListMutator = {
		replace(updated: Task) {
			hydrate(updated);
		},
		remove(_id: number) {
			void goto(resolve('/inbox'));
		}
	};

	async function addSubtask(): Promise<void> {
		const trimmed = newSubtaskTitle.trim();
		if (!trimmed || !task || creatingSubtask) return;
		creatingSubtask = true;
		try {
			const created = await tasksApi.createSubtask(getApiClient(), task.id, { title: trimmed });
			subtasks.items = [...subtasks.items, created];
			newSubtaskTitle = '';
			subtaskInputEl?.focus();
		} catch (err) {
			toast.error(describeError(err, 'Failed to add subtask'));
		} finally {
			creatingSubtask = false;
		}
	}

	function onSubtaskKeydown(e: KeyboardEvent): void {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			void addSubtask();
		}
	}

	function toggleLabel(id: string, name: string, isAuto: boolean): void {
		if (labelIds.includes(id)) {
			labelIds = labelIds.filter((x) => x !== id);
			if (isAuto && !removedAuto.includes(name)) removedAuto = [...removedAuto, name];
		} else {
			labelIds = [...labelIds, id];
			if (isAuto) removedAuto = removedAuto.filter((n) => n !== name);
		}
		scheduleSave();
	}

	function setDate(value: string): void {
		dueDate = dueDate === value ? '' : value;
		scheduleSave();
	}

	function openDatePicker(): void {
		const el = dateInputEl;
		if (!el) return;
		if (typeof el.showPicker === 'function') el.showPicker();
		else el.focus();
	}

	async function save(): Promise<void> {
		if (!task || saving || !title.trim()) return;
		saving = true;
		try {
			const dueAt: string | null = dueDate
				? toIsoUtc(dayStartUtcInTz(dueDate, configStore.value?.timezone ?? null))
				: null;
			const payload: TaskInput = {
				title: title.trim(),
				description,
				priority,
				dayPart,
				dueAt,
				dueHasTime: false,
				recurrenceRule: recurrence,
				labels: labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n),
				removedAutoLabels: removedAuto
			};
			const updated = await tasksApi.update(getApiClient(), task.id, payload);
			hydrate(updated);
		} catch (err) {
			toast.error(describeError(err, 'Failed to save task'));
		} finally {
			saving = false;
		}
	}

	function back(): void {
		if (history.length > 1) history.back();
		else void goto(resolve('/inbox'));
	}

	$effect(() => {
		if (Number.isFinite(taskId)) void loader.refetch();
	});
</script>

<header class="flex items-center justify-between gap-3 border-b border-border px-2 py-1 sm:px-5">
	<div class="flex min-w-0 items-center gap-2">
		<Button variant="ghost" size="sm" onclick={back} class="h-7 shrink-0 gap-1 px-2 text-[10px] uppercase tracking-wider">
			<ArrowLeftIcon class="size-3" />
			Back
		</Button>
		{#if project}
			<a
				href={resolve('/(app)/project/[id]', { id: String(project.id) })}
				class="inline-flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
			>
				<span
					class="size-2 shrink-0 rounded-full"
					style={`background-color: ${project.color}`}
				></span>
				<span class="truncate">{project.title}</span>
			</a>
		{/if}
	</div>
	{#if task}
		<TaskActionsMenu task={task} mutator={pageMutator} />
	{/if}
</header>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !task}
	<div class="px-6 py-8 text-sm text-muted-foreground">Task not found</div>
{:else}
	<form
		onsubmit={(e) => {
			e.preventDefault();
		}}
		class="grid gap-8 p-6 sm:grid-cols-[1fr_16rem] sm:p-8"
	>
		<div class="flex min-w-0 flex-col gap-4">
			<input
				bind:value={title}
				aria-label="Title"
				placeholder="Task name"
				oninput={scheduleSave}
				class="w-full bg-transparent text-xl font-semibold leading-tight outline-none placeholder:text-muted-foreground/60"
			/>
			<div class="relative">
				{#if !description && !descriptionFocused}
					<TextAlignStartIcon class="pointer-events-none absolute left-0 top-[2px] size-3.5 text-muted-foreground/40" />
				{/if}
				<textarea
					bind:value={description}
					aria-label="Description"
					placeholder="Description"
					rows="10"
					oninput={scheduleSave}
					onfocus={() => (descriptionFocused = true)}
					onblur={() => (descriptionFocused = false)}
					class="w-full resize-y rounded-md border border-transparent bg-transparent text-sm leading-relaxed outline-none transition-colors placeholder:text-muted-foreground/60 focus:border-border focus:bg-muted/30 focus:p-3"
					class:pl-5={!description && !descriptionFocused}
				></textarea>
			</div>

			{#if task.inboxId === null}
				<section class="flex flex-col gap-2">
					<div class="flex items-baseline justify-between gap-2">
						<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
							Subtasks
						</span>
						{#if subtasks.items.length > 0}
							<span class="text-[11px] text-muted-foreground/70">{subtasks.items.length}</span>
						{/if}
					</div>
					{#if subtasks.items.length > 0}
						<div class="rounded-md border border-border/60">
							<TaskTree
								tasks={subtasks.items}
								showProject={false}
								mutator={subtasks.mutator}
								onToggle={(t) => toggleComplete(t, subtasks.mutator, { removeWhenCompleted: false })}
							/>
						</div>
					{/if}
					<div class="flex items-center gap-2 rounded-md border border-dashed border-border/70 bg-muted/20 px-2.5 py-1.5 transition-colors focus-within:border-border focus-within:bg-muted/40">
						<PlusIcon class="size-3.5 shrink-0 text-muted-foreground" />
						<input
							bind:this={subtaskInputEl}
							bind:value={newSubtaskTitle}
							onkeydown={onSubtaskKeydown}
							disabled={creatingSubtask}
							placeholder="Add subtask"
							aria-label="Add subtask"
							class="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground/60 disabled:opacity-60"
						/>
						{#if newSubtaskTitle.trim()}
							<Button
								type="button"
								size="xs"
								variant="secondary"
								onclick={() => void addSubtask()}
								disabled={creatingSubtask}
							>
								Add
							</Button>
						{/if}
					</div>
				</section>
			{/if}
		</div>

		<aside class="flex flex-col gap-5 sm:border-l sm:border-border sm:pl-6">
			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Date
				</span>
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
					<button
						type="button"
						onclick={openDatePicker}
						aria-pressed={isCustomDate}
						aria-label="Custom date"
						title={isCustomDate ? dueDate : 'Pick a date'}
						class="relative inline-flex h-7 items-center gap-1 rounded-[5px] px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
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
							tabindex="-1"
							aria-hidden="true"
							onchange={scheduleSave}
							class="pointer-events-none absolute inset-0 size-full opacity-0"
						/>
					</button>
				</div>
			</div>

			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Priority
				</span>
				<PriorityPicker bind:value={priority} />
			</div>

			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Day part
				</span>
				<DayPartPicker bind:value={dayPart} />
			</div>

			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Repeat
				</span>
				<RecurrencePicker bind:value={recurrence} />
			</div>

			{#if allLabels.length > 0}
				<div class="flex flex-col gap-1.5">
					<span
						class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
					>
						Labels
					</span>
					{#if selectedLabels.length > 0}
						<div class="flex flex-wrap gap-1">
							{#each selectedLabels as label (label.id)}
								<button
									type="button"
									onclick={() =>
										toggleLabel(String(label.id), label.name, autoLabelNames.has(label.name))}
									class="group/chip inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium text-foreground transition-opacity hover:opacity-80"
									style={label.color
										? `background-color: color-mix(in srgb, ${label.color} 32%, transparent);`
										: 'background-color: var(--accent);'}
								>
									{label.name}
									<XIcon class="size-3 opacity-60 transition-opacity group-hover/chip:opacity-100" />
								</button>
							{/each}
						</div>
					{/if}
					<details class="group/labels">
						<summary
							class="inline-flex cursor-pointer list-none items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
						>
							<span>Edit labels</span>
						</summary>
						<div class="mt-2 flex flex-wrap gap-1">
							{#each allLabels as label (label.id)}
								{@const id = String(label.id)}
								{@const active = labelIds.includes(id)}
								<button
									type="button"
									onclick={() => toggleLabel(id, label.name, autoLabelNames.has(label.name))}
									class="rounded-full px-2 py-0.5 text-xs transition-colors"
									class:font-medium={active}
									class:text-foreground={active}
									class:border={!active}
									class:border-border={!active}
									class:text-muted-foreground={!active}
									class:hover:bg-accent={!active}
									style={active && label.color
										? `background-color: color-mix(in srgb, ${label.color} 32%, transparent);`
										: undefined}
								>
									{label.name}
								</button>
							{/each}
						</div>
					</details>
				</div>
			{/if}
		</aside>
	</form>
{/if}
