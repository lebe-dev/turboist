<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import XIcon from 'phosphor-svelte/lib/X';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { configStore } from '$lib/stores/config.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import PriorityPicker from '$lib/components/task/PriorityPicker.svelte';
	import DayPartPicker from '$lib/components/task/DayPartPicker.svelte';
	import { dayKeyInTz, dayStartUtcInTz, parseIso, timeKeyInTz, toIsoUtc } from '$lib/utils/format';
	import { describeError } from '$lib/utils/taskActions';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const taskId = $derived(Number(page.params.id));

	let task = $state<Task | null>(null);
	let notFound = $state(false);
	let saving = $state(false);
	let deleting = $state(false);

	let title = $state('');
	let description = $state('');
	let priority = $state<Priority>('no-priority');
	let dayPart = $state<DayPart>('none');
	let dueDate = $state('');
	let dueTime = $state('');
	let recurrence = $state('');
	let labelIds = $state<string[]>([]);
	let removedAuto = $state<string[]>([]);

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
		task = t;
		title = t.title;
		description = t.description ?? '';
		priority = t.priority;
		dayPart = t.dayPart;
		recurrence = t.recurrenceRule ?? '';
		const dt = parseIso(t.dueAt);
		if (dt) {
			const tz = configStore.value?.timezone ?? null;
			dueDate = dayKeyInTz(dt, tz);
			dueTime = t.dueHasTime ? timeKeyInTz(dt, tz) : '';
		} else {
			dueDate = '';
			dueTime = '';
		}
		labelIds = t.labels.map((l) => String(l.id));
		removedAuto = [];
	}

	const loader = usePageLoad(async (isValid) => {
		notFound = false;
		task = null;
		if (!Number.isFinite(taskId)) {
			notFound = true;
			return;
		}
		const t = await tasksApi.get(getApiClient(), taskId);
		if (!isValid()) return;
		hydrate(t);
	}, {
		autoLoad: false,
		initialLoading: true,
		onError(err) {
			if (err instanceof ApiError && err.code === 'not_found') {
				notFound = true;
			}
			toast.error(describeError(err, 'Failed to load task'));
		}
	});

	function toggleLabel(id: string, name: string, isAuto: boolean): void {
		if (labelIds.includes(id)) {
			labelIds = labelIds.filter((x) => x !== id);
			if (isAuto && !removedAuto.includes(name)) removedAuto = [...removedAuto, name];
		} else {
			labelIds = [...labelIds, id];
			if (isAuto) removedAuto = removedAuto.filter((n) => n !== name);
		}
	}

	async function save(): Promise<void> {
		if (!task || saving || !title.trim()) return;
		saving = true;
		try {
			let dueAt: string | null = null;
			let dueHasTime = false;
			if (dueDate) {
				const tz = configStore.value?.timezone ?? null;
				if (dueTime) {
					const [hh, mm] = dueTime.split(':').map(Number);
					const start = dayStartUtcInTz(dueDate, tz);
					dueAt = toIsoUtc(new Date(start.getTime() + (hh * 60 + mm) * 60000));
					dueHasTime = true;
				} else {
					dueAt = toIsoUtc(dayStartUtcInTz(dueDate, tz));
					dueHasTime = false;
				}
			}
			const payload: TaskInput = {
				title: title.trim(),
				description,
				priority,
				dayPart,
				dueAt,
				dueHasTime,
				recurrenceRule: recurrence.trim() ? recurrence.trim() : null,
				labels: labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n),
				removedAutoLabels: removedAuto
			};
			const updated = await tasksApi.update(getApiClient(), task.id, payload);
			hydrate(updated);
			toast.success('Saved');
		} catch (err) {
			toast.error(describeError(err, 'Failed to save task'));
		} finally {
			saving = false;
		}
	}

	async function remove(): Promise<void> {
		if (!task || deleting) return;
		if (!confirm(`Delete "${task.title}"? Subtasks will also be removed.`)) return;
		deleting = true;
		try {
			await tasksApi.remove(getApiClient(), task.id);
			toast.success('Task deleted');
			void goto(resolve('/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to delete task'));
			deleting = false;
		}
	}

	function back(): void {
		if (history.length > 1) history.back();
		else void goto(resolve('/inbox'));
	}

	$effect(() => {
		void loader.refetch();
	});
</script>

<header class="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-8">
	<Button variant="ghost" size="sm" onclick={back} class="gap-2">
		<ArrowLeftIcon class="size-4" />
		Back
	</Button>
	<div class="flex items-center gap-2">
		{#if task}
			<Button
				variant="ghost"
				size="sm"
				onclick={remove}
				disabled={deleting}
				class="gap-2 text-destructive hover:bg-destructive/10 hover:text-destructive"
			>
				<TrashIcon class="size-4" />
				Delete
			</Button>
		{/if}
		<Button size="sm" onclick={save} disabled={!task || saving || !title.trim()}>
			{saving ? 'Saving…' : 'Save'}
		</Button>
	</div>
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
			void save();
		}}
		class="grid gap-8 p-6 sm:grid-cols-[1fr_16rem] sm:p-8"
	>
		<div class="flex min-w-0 flex-col gap-4">
			<input
				bind:value={title}
				aria-label="Title"
				placeholder="Task name"
				class="w-full bg-transparent text-2xl font-semibold leading-tight outline-none placeholder:text-muted-foreground/60"
			/>
			<textarea
				bind:value={description}
				aria-label="Description"
				placeholder="Description"
				rows="10"
				class="w-full resize-y rounded-md border border-transparent bg-transparent text-sm leading-relaxed outline-none transition-colors placeholder:text-muted-foreground/60 focus:border-border focus:bg-muted/30 focus:p-3"
			></textarea>
		</div>

		<aside class="flex flex-col gap-5 sm:border-l sm:border-border sm:pl-6">
			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Date
				</span>
				<Input type="date" bind:value={dueDate} class="h-8 text-xs" />
				<Input type="time" bind:value={dueTime} class="h-8 text-xs" />
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
				<DayPartPicker bind:value={dayPart} compact />
			</div>

			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					Recurrence
				</span>
				<Input bind:value={recurrence} placeholder="FREQ=DAILY" class="h-8 text-xs" />
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
									class="group/chip inline-flex items-center gap-1 rounded-full bg-accent px-2 py-0.5 text-xs font-medium text-accent-foreground transition-colors hover:bg-accent/70"
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
									class="rounded-full border px-2 py-0.5 text-xs transition-colors"
									class:bg-primary={active}
									class:text-primary-foreground={active}
									class:border-primary={active}
									class:border-border={!active}
									class:hover:bg-accent={!active}
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
