<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Label } from '$lib/components/ui/label';
	import { toast } from 'svelte-sonner';
	import { getApiClient } from '$lib/api/client';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import type { Project } from '$lib/api/types';
	import ColorPicker from './ColorPicker.svelte';
	import { DEFAULT_COLOR } from './colorPalette';
	import { useFormDialog } from '$lib/hooks/useFormDialog.svelte';

	let {
		open = $bindable(false),
		initial = null,
		defaultContextId = null,
		onSaved
	}: {
		open?: boolean;
		initial?: Project | null;
		defaultContextId?: number | null;
		onSaved?: (project: Project) => void;
	} = $props();

	let title = $state('');
	let description = $state('');
	let color = $state<string>(DEFAULT_COLOR);
	let contextId = $state<string>('');
	let labelIds = $state<string[]>([]);

	const form = useFormDialog();

	const allContexts = $derived(contextsStore.items);
	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);

	$effect(() => {
		if (open) {
			title = initial?.title ?? '';
			description = initial?.description ?? '';
			color = initial?.color ?? DEFAULT_COLOR;
			const fallback = defaultContextId ?? allContexts[0]?.id ?? null;
			contextId = String(initial?.contextId ?? fallback ?? '');
			labelIds = (initial?.labels ?? []).map((l) => String(l.id));
		}
	});

	async function submit(e: Event) {
		e.preventDefault();
		if (!title.trim()) return;
		const ctxIdNum = Number(contextId);
		if (!Number.isFinite(ctxIdNum) || ctxIdNum <= 0) {
			toast.error('Pick a context');
			return;
		}
		const saved = await form.submit(
			async () => {
				const client = getApiClient();
				const labelNames = labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n);

				if (initial) {
					return projectsApi.update(client, initial.id, {
						title: title.trim(),
						description: description.trim() || null,
						color,
						contextId: ctxIdNum,
						labels: labelNames
					});
				} else {
					return contextsApi.createProject(client, ctxIdNum, {
						title: title.trim(),
						description: description.trim() || null,
						color,
						labels: labelNames
					});
				}
			},
			{ success: initial ? 'Project updated' : 'Project created', error: 'Failed to save project' }
		);
		if (saved) {
			projectsStore.upsert(saved);
			onSaved?.(saved);
			open = false;
		}
	}

	const ctxLabel = $derived(
		allContexts.find((c) => String(c.id) === contextId)?.name ?? 'Pick context'
	);
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-lg">
		<Sheet.Header>
			<Sheet.Title>{initial ? 'Edit project' : 'New project'}</Sheet.Title>
			<Sheet.Description>Projects live inside a context.</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 overflow-y-auto px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="prj-title">Title</Label>
				<Input id="prj-title" bind:value={title} placeholder="Website redesign" autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<Label for="prj-desc">Description</Label>
				<Textarea id="prj-desc" rows={3} bind:value={description} />
			</div>

			<div class="flex flex-col gap-1">
				<Label>Context</Label>
				<Select.Root type="single" bind:value={contextId}>
					<Select.Trigger>{ctxLabel}</Select.Trigger>
					<Select.Content>
						{#each allContexts as ctx (ctx.id)}
							<Select.Item value={String(ctx.id)}>{ctx.name}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>

			<div class="flex flex-col gap-1">
				<Label>Color</Label>
				<ColorPicker bind:value={color} />
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
				<Button type="submit" disabled={!title.trim() || form.submitting}>
					{form.submitting ? 'Saving…' : initial ? 'Save' : 'Create'}
				</Button>
				<Sheet.Close>Cancel</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
