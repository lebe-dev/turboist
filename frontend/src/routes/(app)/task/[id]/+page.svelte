<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import XIcon from 'phosphor-svelte/lib/X';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import TextAlignStartIcon from 'phosphor-svelte/lib/TextAlignLeft';
	import { Button } from '$lib/components/ui/button';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { configStore } from '$lib/stores/config.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import PriorityPicker from '$lib/components/task/PriorityPicker.svelte';
	import DayPartPicker from '$lib/components/task/DayPartPicker.svelte';
	import RecurrencePicker from '$lib/components/task/RecurrencePicker.svelte';
	import TaskActionsMenu from '$lib/components/task/TaskActionsMenu.svelte';
	import { dayKeyInTz, dayStartUtcInTz, parseIso, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { describeError } from '$lib/utils/taskActions';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const taskId = $derived(Number(page.params.id));

	let task = $state<Task | null>(null);
	let notFound = $state(false);
	let saving = $state(false);

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
			if (!Number.isFinite(taskId)) {
				notFound = true;
				return;
			}
			const t = await tasksApi.get(getApiClient(), taskId);
			if (!isValid()) return;
			hydrate(t);
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
	<Button variant="ghost" size="sm" onclick={back} class="h-7 gap-1 px-2 text-[10px] uppercase tracking-wider">
		<ArrowLeftIcon class="size-3" />
		Back
	</Button>
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
