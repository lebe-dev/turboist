<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Label } from '$lib/components/ui/label';
	import type { Task } from '$lib/api/types';
	import { decomposeTask, type ListMutator } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		task,
		mutator
	}: {
		open?: boolean;
		task: Task | null;
		mutator: ListMutator;
	} = $props();

	let value = $state('');
	let submitting = $state(false);

	$effect(() => {
		if (open) {
			value = '';
			submitting = false;
		}
	});

	const titles = $derived(
		value
			.split('\n')
			.map((s) => s.trim())
			.filter((s) => s.length > 0)
	);

	async function submit(e: Event) {
		e.preventDefault();
		if (!task || titles.length === 0 || submitting) return;
		submitting = true;
		try {
			const ok = await decomposeTask(task, titles, mutator);
			if (ok) open = false;
		} finally {
			submitting = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{$t('dialog.decompose.title')}</Sheet.Title>
			<Sheet.Description>{$t('dialog.decompose.description')}</Sheet.Description>
		</Sheet.Header>

		<form class="flex flex-col gap-3 px-4 py-2" onsubmit={submit}>
			<div class="flex flex-col gap-1">
				<Label for="decompose-titles">{$t('dialog.decompose.label')}</Label>
				<Textarea
					id="decompose-titles"
					bind:value
					placeholder={$t('dialog.decompose.placeholder')}
					rows={8}
					autofocus
				/>
				<div class="text-xs text-muted-foreground">
					{$t('dialog.decompose.count', { values: { count: titles.length } })}
				</div>
			</div>

			<Sheet.Footer class="px-0">
				<Button type="submit" disabled={titles.length === 0 || submitting}>
					{submitting ? $t('common.saving') : $t('dialog.decompose.submit')}
				</Button>
				<Sheet.Close>{$t('common.cancel')}</Sheet.Close>
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
