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
	import { t } from '$lib/i18n';

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
			{
				success: initial ? $t('dialog.context.updated') : $t('dialog.context.created'),
				error: $t('dialog.context.failedSave')
			}
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
			<Sheet.Title>{initial ? $t('dialog.context.editTitle') : $t('dialog.context.newTitle')}</Sheet.Title>
			<Sheet.Description>{$t('dialog.context.description')}</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="ctx-name">{$t('common.name')}</Label>
				<Input id="ctx-name" bind:value={name} placeholder={$t('dialog.context.namePlaceholder')} autofocus />
			</div>

			<div class="flex flex-col gap-1">
				<Label>{$t('common.color')}</Label>
				<ColorPicker bind:value={color} />
			</div>

			<div class="flex items-center justify-between">
				<Label for="ctx-fav">{$t('common.favourite')}</Label>
				<Switch id="ctx-fav" bind:checked={isFavourite} />
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
