<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { getApiClient } from '$lib/api/client';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { sections as sectionsApi } from '$lib/api/endpoints/sections';
	import type { ProjectSection } from '$lib/api/types';
	import { useFormDialog } from '$lib/hooks/useFormDialog.svelte';

	let {
		open = $bindable(false),
		initial = null,
		projectId,
		onSaved
	}: {
		open?: boolean;
		initial?: ProjectSection | null;
		projectId: number;
		onSaved?: (section: ProjectSection) => void;
	} = $props();

	let title = $state('');

	const form = useFormDialog();

	$effect(() => {
		if (open) title = initial?.title ?? '';
	});

	async function submit(e: Event) {
		e.preventDefault();
		if (!title.trim()) return;
		const saved = await form.submit(
			async () => {
				const client = getApiClient();
				return initial
					? await sectionsApi.update(client, initial.id, { title: title.trim() })
					: await projectsApi.createSection(client, projectId, { title: title.trim() });
			},
			{ success: initial ? 'Section updated' : 'Section created', error: 'Failed to save section' }
		);
		if (saved) {
			onSaved?.(saved);
			open = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{initial ? 'Rename section' : 'New section'}</Sheet.Title>
			<Sheet.Description>Sections group tasks within a project.</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="sec-title">Title</Label>
				<Input id="sec-title" bind:value={title} placeholder="Backlog" autofocus />
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={!title.trim() || form.submitting}>
					{form.submitting ? 'Saving…' : initial ? 'Save' : 'Create'}
				</Button>
				<Sheet.Close>Cancel</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
