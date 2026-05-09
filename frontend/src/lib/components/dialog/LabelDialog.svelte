<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label as FormLabel } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { getApiClient } from '$lib/api/client';
	import { labels as labelsApi } from '$lib/api/endpoints/labels';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import type { Label } from '$lib/api/types';
	import ColorPicker from './ColorPicker.svelte';
	import { DEFAULT_COLOR } from './colorPalette';
	import { useFormDialog } from '$lib/hooks/useFormDialog.svelte';
	import { t } from '$lib/i18n';

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
					? await labelsApi.update(client, initial.id, payload)
					: await labelsApi.create(client, payload);
			},
			{
				success: initial ? $t('dialog.label.updated') : $t('dialog.label.created'),
				error: $t('dialog.label.failedSave')
			}
		);
		if (saved) {
			labelsStore.upsert(saved);
			onSaved?.(saved);
			open = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{initial ? $t('dialog.label.editTitle') : $t('dialog.label.newTitle')}</Sheet.Title>
			<Sheet.Description>{$t('dialog.label.description')}</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<FormLabel for="lbl-name">{$t('common.name')}</FormLabel>
				<Input id="lbl-name" bind:value={name} placeholder={$t('dialog.label.namePlaceholder')} autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<FormLabel>{$t('common.color')}</FormLabel>
				<ColorPicker bind:value={color} />
			</div>

			<div class="flex items-center justify-between">
				<FormLabel for="lbl-fav">{$t('common.favourite')}</FormLabel>
				<Switch id="lbl-fav" bind:checked={isFavourite} />
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={!name.trim() || form.submitting}>
					{form.submitting ? $t('common.saving') : initial ? $t('common.save') : $t('common.create')}
				</Button>
				<Sheet.Close>{$t('common.cancel')}</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
