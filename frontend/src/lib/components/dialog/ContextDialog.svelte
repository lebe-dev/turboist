<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { getApiClient } from '$lib/api/client';
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import type { Context } from '$lib/api/types';
	import ColorPicker from './ColorPicker.svelte';
	import { DEFAULT_COLOR } from './colorPalette';
	import { useFormDialog } from '$lib/hooks/useFormDialog.svelte';

	let {
		open = $bindable(false),
		initial = null,
		onSaved
	}: {
		open?: boolean;
		initial?: Context | null;
		onSaved?: (ctx: Context) => void;
	} = $props();

	let name = $state('');
	let color = $state<string>(DEFAULT_COLOR);
	let isFavourite = $state(false);

	const form = useFormDialog();

	$effect(() => {
		if (open) {
			name = initial?.name ?? '';
			color = initial?.color ?? DEFAULT_COLOR;
			isFavourite = initial?.isFavourite ?? false;
		}
	});

	async function submit(e: Event) {
		e.preventDefault();
		if (!name.trim()) return;
		const saved = await form.submit(
			async () => {
				const client = getApiClient();
				const payload = { name: name.trim(), color, isFavourite };
				return initial
					? await contextsApi.update(client, initial.id, payload)
					: await contextsApi.create(client, payload);
			},
			{ success: initial ? 'Context updated' : 'Context created', error: 'Failed to save context' }
		);
		if (saved) {
			contextsStore.upsert(saved);
			onSaved?.(saved);
			open = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{initial ? 'Edit context' : 'New context'}</Sheet.Title>
			<Sheet.Description>Group related projects under a context.</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="ctx-name">Name</Label>
				<Input id="ctx-name" bind:value={name} placeholder="Personal" autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<Label>Color</Label>
				<ColorPicker bind:value={color} />
			</div>

			<div class="flex items-center justify-between">
				<Label for="ctx-fav">Favourite</Label>
				<Switch id="ctx-fav" bind:checked={isFavourite} />
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={!name.trim() || form.submitting}>
					{form.submitting ? 'Saving…' : initial ? 'Save' : 'Create'}
				</Button>
				<Sheet.Close>Cancel</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
