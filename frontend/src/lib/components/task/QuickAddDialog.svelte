<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import type { Priority, TaskInput } from '$lib/api/types';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import PriorityPicker from './PriorityPicker.svelte';
	import { toIsoUtc } from '$lib/utils/format';

	let {
		open = $bindable(false),
		defaultProjectId = null,
		onSubmit
	}: {
		open?: boolean;
		defaultProjectId?: number | null;
		onSubmit?: (
			payload: TaskInput,
			target: { projectId: number | null; labels: string[] }
		) => void | Promise<void>;
	} = $props();

	let title = $state('');
	let priority = $state<Priority>('no-priority');
	let dueDate = $state('');
	// svelte-ignore state_referenced_locally
	let projectId = $state<string>(defaultProjectId ? String(defaultProjectId) : '');
	let labelIds = $state<string[]>([]);
	let submitting = $state(false);

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);

	function reset() {
		title = '';
		priority = 'no-priority';
		dueDate = '';
		projectId = defaultProjectId ? String(defaultProjectId) : '';
		labelIds = [];
	}

	async function submit(e: Event) {
		e.preventDefault();
		if (!title.trim() || submitting) return;
		submitting = true;
		try {
			const payload: TaskInput = {
				title: title.trim(),
				priority,
				dueAt: dueDate ? toIsoUtc(new Date(`${dueDate}T00:00:00`)) : null,
				dueHasTime: false,
				labels: labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n)
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
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Quick add task</Sheet.Title>
			<Sheet.Description>Title plus optional project, priority, due date, labels.</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="qa-title">Title</Label>
				<Input id="qa-title" bind:value={title} placeholder="Buy milk" autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<Label for="qa-due">Due date</Label>
				<Input id="qa-due" type="date" bind:value={dueDate} />
			</div>

			<div class="flex flex-col gap-1">
				<Label>Priority</Label>
				<PriorityPicker bind:value={priority} />
			</div>

			<div class="flex flex-col gap-1">
				<Label>Project</Label>
				<Select.Root type="single" bind:value={projectId}>
					<Select.Trigger>{projectsStore.items.find((p) => String(p.id) === projectId)?.title ?? 'Inbox'}</Select.Trigger>
					<Select.Content>
						<Select.Item value="">Inbox</Select.Item>
						{#each projectsStore.items as project (project.id)}
							<Select.Item value={String(project.id)}>{project.title}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
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
				<Button type="submit" disabled={!title.trim() || submitting}>Add task</Button>
				<Sheet.Close>Cancel</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
