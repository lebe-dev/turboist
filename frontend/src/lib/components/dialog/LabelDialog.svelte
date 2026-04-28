<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label as FormLabel } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { getApiClient } from '$lib/api/client';
	import { labels as labelsApi } from '$lib/api/endpoints/labels';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import type { Label } from '$lib/api/types';
	import ColorPicker from './ColorPicker.svelte';
	import { DEFAULT_COLOR } from './colorPalette';

	let {
		open = $bindable(false),
		initial = null,
		onSaved
	}: {
		open?: boolean;
		initial?: Label | null;
		onSaved?: (label: Label) => void;
	} = $props();

	let name = $state('');
	let color = $state<string>(DEFAULT_COLOR);
	let isFavourite = $state(false);
	let submitting = $state(false);

	$effect(() => {
		if (open) {
			name = initial?.name ?? '';
			color = initial?.color ?? DEFAULT_COLOR;
			isFavourite = initial?.isFavourite ?? false;
		}
	});

	async function submit(e: Event) {
		e.preventDefault();
		if (!name.trim() || submitting) return;
		submitting = true;
		try {
			const client = getApiClient();
			const payload = { name: name.trim(), color, isFavourite };
			const saved = initial
				? await labelsApi.update(client, initial.id, payload)
				: await labelsApi.create(client, payload);
			labelsStore.upsert(saved);
			onSaved?.(saved);
			toast.success(initial ? 'Label updated' : 'Label created');
			open = false;
		} catch (err) {
			toast.error(describeError(err, 'Failed to save label'));
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{initial ? 'Edit label' : 'New label'}</Sheet.Title>
			<Sheet.Description>Tag tasks across projects.</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<FormLabel for="lbl-name">Name</FormLabel>
				<Input id="lbl-name" bind:value={name} placeholder="urgent" autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<FormLabel>Color</FormLabel>
				<ColorPicker bind:value={color} />
			</div>

			<div class="flex items-center justify-between">
				<FormLabel for="lbl-fav">Favourite</FormLabel>
				<Switch id="lbl-fav" bind:checked={isFavourite} />
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={!name.trim() || submitting}>
					{submitting ? 'Saving…' : initial ? 'Save' : 'Create'}
				</Button>
				<Sheet.Close>Cancel</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
