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
	import { t } from '$lib/i18n';

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
			{
				success: initial ? $t('dialog.section.updated') : $t('dialog.section.created'),
				error: $t('dialog.section.failedSave')
			}
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
			<Sheet.Title>{initial ? $t('dialog.section.renameTitle') : $t('dialog.section.newTitle')}</Sheet.Title>
			<Sheet.Description>{$t('dialog.section.description')}</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="sec-title">{$t('common.title')}</Label>
				<Input id="sec-title" bind:value={title} placeholder={$t('dialog.section.titlePlaceholder')} autofocus />
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={!title.trim() || form.submitting}>
					{form.submitting ? $t('common.saving') : initial ? $t('common.save') : $t('common.create')}
				</Button>
				<Sheet.Close>{$t('common.cancel')}</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
