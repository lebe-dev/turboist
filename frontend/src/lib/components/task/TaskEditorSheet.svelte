<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Label } from '$lib/components/ui/label';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import PriorityPicker from './PriorityPicker.svelte';
	import DayPartPicker from './DayPartPicker.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { parseIso, toIsoUtc } from '$lib/utils/format';

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

	$effect(() => {
		if (!task) return;
		title = task.title;
		description = task.description ?? '';
		priority = task.priority;
		dayPart = task.dayPart;
		recurrence = task.recurrenceRule ?? '';
		const dt = parseIso(task.dueAt);
		if (dt) {
			const pad = (n: number) => String(n).padStart(2, '0');
			dueDate = `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`;
			dueTime = task.dueHasTime ? `${pad(dt.getHours())}:${pad(dt.getMinutes())}` : '';
		} else {
			dueDate = '';
			dueTime = '';
		}
		labelIds = task.labels.map((l) => String(l.id));
		removedAuto = [];
	});

	function submit(e: Event) {
		e.preventDefault();
		if (!task || submitting) return;
		submitting = true;
		try {
			let dueAt: string | null = null;
			let dueHasTime = false;
			if (dueDate) {
				if (dueTime) {
					dueAt = toIsoUtc(new Date(`${dueDate}T${dueTime}:00`));
					dueHasTime = true;
				} else {
					dueAt = toIsoUtc(new Date(`${dueDate}T00:00:00Z`));
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
			void onSubmit?.(task.id, payload, { removedAutoLabels: removedAuto });
			open = false;
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-lg">
		<Sheet.Header>
			<Sheet.Title>Edit task</Sheet.Title>
			<Sheet.Description>Update task fields.</Sheet.Description>
		</Sheet.Header>

		{#if task}
			<form class="flex flex-col gap-3 overflow-y-auto px-4 py-2" onsubmit={submit}>
				<div class="flex flex-col gap-1">
					<Label for="ed-title">Title</Label>
					<Input id="ed-title" bind:value={title} />
				</div>

				<div class="flex flex-col gap-1">
					<Label for="ed-desc">Description</Label>
					<Textarea id="ed-desc" rows={4} bind:value={description} />
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div class="flex flex-col gap-1">
						<Label for="ed-due">Due date</Label>
						<Input id="ed-due" type="date" bind:value={dueDate} />
					</div>
					<div class="flex flex-col gap-1">
						<Label for="ed-time">Due time</Label>
						<Input id="ed-time" type="time" bind:value={dueTime} />
					</div>
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div class="flex flex-col gap-1">
						<Label>Priority</Label>
						<PriorityPicker bind:value={priority} />
					</div>
					<div class="flex flex-col gap-1">
						<Label>Day part</Label>
						<DayPartPicker bind:value={dayPart} />
					</div>
				</div>

				<div class="flex flex-col gap-1">
					<Label for="ed-rrule">Recurrence (RRULE)</Label>
					<Input id="ed-rrule" bind:value={recurrence} placeholder="FREQ=DAILY" />
				</div>

				{#if allLabels.length > 0}
					<div class="flex flex-col gap-1">
						<Label>Labels</Label>
						<div class="flex flex-wrap gap-1">
							{#each allLabels as label (label.id)}
								{@const id = String(label.id)}
								{@const active = labelIds.includes(id)}
								<button
									type="button"
									class="rounded border px-2 py-0.5 text-xs"
									class:bg-primary={active}
									class:text-primary-foreground={active}
									onclick={() =>
										(labelIds = active
											? labelIds.filter((x) => x !== id)
											: [...labelIds, id])}
								>
									@{label.name}
								</button>
							{/each}
						</div>
					</div>
				{/if}

				<Sheet.Footer class="px-0">
					<Button type="submit" disabled={submitting}>Save</Button>
					<Sheet.Close>Cancel</Sheet.Close>
				</Sheet.Footer>
			</form>
		{/if}
	</Sheet.Content>
</Sheet.Root>
