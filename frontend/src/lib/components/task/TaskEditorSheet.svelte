<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import PriorityPicker from './PriorityPicker.svelte';
	import DayPartPicker from './DayPartPicker.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { dayKeyInTz, dayStartUtcInTz, parseIso, timeKeyInTz, toIsoUtc } from '$lib/utils/format';
	import XIcon from 'phosphor-svelte/lib/X';

	let {
		open = $bindable(false),
		task,
		onSubmit
	}: {
		open?: boolean;
		task: Task | null;
		onSubmit?: (
			id: number,
			payload: TaskInput,
			extras: { removedAutoLabels: string[] }
		) => void | Promise<void>;
	} = $props();

	let title = $state('');
	let description = $state('');
	let priority = $state<Priority>('no-priority');
	let dayPart = $state<DayPart>('none');
	let dueDate = $state('');
	let dueTime = $state('');
	let recurrence = $state('');
	let labelIds = $state<string[]>([]);
	let removedAuto = $state<string[]>([]);
	let submitting = $state(false);

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);
	const autoLabelNames = $derived(
		new Set((configStore.value?.autoLabels ?? []).map((r) => r.label))
	);
	const selectedLabels = $derived(
		labelIds
			.map((id) => allLabels.find((l) => String(l.id) === id))
			.filter((l): l is (typeof allLabels)[number] => !!l)
	);

	$effect(() => {
		if (!task) return;
		title = task.title;
		description = task.description ?? '';
		priority = task.priority;
		dayPart = task.dayPart;
		recurrence = task.recurrenceRule ?? '';
		const dt = parseIso(task.dueAt);
		if (dt) {
			const tz = configStore.value?.timezone ?? null;
			dueDate = dayKeyInTz(dt, tz);
			dueTime = task.dueHasTime ? timeKeyInTz(dt, tz) : '';
		} else {
			dueDate = '';
			dueTime = '';
		}
		labelIds = task.labels.map((l) => String(l.id));
		removedAuto = [];
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

	async function submit(e: Event) {
		e.preventDefault();
		if (!task || submitting || !title.trim()) return;
		submitting = true;
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
			try {
				await onSubmit?.(task.id, payload, { removedAutoLabels: removedAuto });
				open = false;
			} catch {
				// keep the sheet open so the user can retry without losing edits
			}
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full p-0 data-[side=right]:sm:max-w-3xl">
		<Sheet.Header class="border-b border-border px-6 py-4">
			<Sheet.Title class="text-base font-semibold">Edit task</Sheet.Title>
			<Sheet.Description class="sr-only">Update task fields.</Sheet.Description>
		</Sheet.Header>

		{#if task}
			<form
				onsubmit={submit}
				class="flex h-[calc(100%-12rem)] flex-col gap-6 overflow-y-auto p-6 sm:grid sm:h-[calc(100%-9rem)] sm:grid-cols-[1fr_14rem] sm:gap-8"
			>
				<div class="flex min-w-0 flex-col gap-4">
					<input
						bind:value={title}
						aria-label="Title"
						placeholder="Task name"
						class="w-full bg-transparent text-xl font-semibold leading-tight outline-none placeholder:text-muted-foreground/60"
					/>
					<textarea
						bind:value={description}
						aria-label="Description"
						placeholder="Description"
						rows="6"
						class="w-full resize-none rounded-md border border-transparent bg-transparent text-sm leading-relaxed outline-none transition-colors placeholder:text-muted-foreground/60 focus:border-border focus:bg-muted/30 focus:p-3"
					></textarea>
				</div>

				<aside class="flex flex-col gap-5 sm:border-l sm:border-border sm:pl-6">
					<div class="flex flex-col gap-1.5">
						<span
							class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
						>
							Date
						</span>
						<Input type="date" bind:value={dueDate} class="h-8 text-xs" />
						<Input type="time" bind:value={dueTime} class="h-8 text-xs" />
					</div>

					<div class="flex flex-col gap-1.5">
						<span
							class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
						>
							Priority
						</span>
						<PriorityPicker bind:value={priority} />
					</div>

					<div class="flex flex-col gap-1.5">
						<span
							class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
						>
							Day part
						</span>
						<DayPartPicker bind:value={dayPart} compact />
					</div>

					<div class="flex flex-col gap-1.5">
						<span
							class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
						>
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
												toggleLabel(
													String(label.id),
													label.name,
													autoLabelNames.has(label.name)
												)}
											class="group/chip inline-flex items-center gap-1 rounded-full bg-accent px-2 py-0.5 text-xs font-medium text-accent-foreground transition-colors hover:bg-accent/70"
										>
											{#if label.color}
												<span
													class="size-1.5 rounded-full"
													style={`background-color: ${label.color}`}
												></span>
											{/if}
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

			<Sheet.Footer
				class="flex shrink-0 flex-row items-center justify-end gap-2 border-t border-border bg-background px-6 py-3"
			>
				<Sheet.Close>
					{#snippet child({ props })}
						<Button {...props} variant="ghost" size="sm" type="button">Cancel</Button>
					{/snippet}
				</Sheet.Close>
				<Button type="button" size="sm" disabled={submitting || !title.trim()} onclick={submit}>
					Save
				</Button>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
