<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import { t } from '$lib/i18n';
	import XIcon from 'phosphor-svelte/lib/X';
	import StackIcon from 'phosphor-svelte/lib/Stack';

	let {
		onGroup,
		busy = false
	}: {
		onGroup: () => void;
		busy?: boolean;
	} = $props();

	const visible = $derived(taskSelectionStore.mode && taskSelectionStore.count >= 1);
	const canGroup = $derived(taskSelectionStore.count >= 2);
</script>

{#if visible}
	<div
		class="fixed bottom-4 left-1/2 z-40 flex -translate-x-1/2 items-center gap-2 rounded-full border border-border bg-popover px-3 py-2 text-popover-foreground shadow-xl"
		role="region"
		aria-label={$t('selection.bar.aria')}
	>
		<span class="text-sm font-medium">
			{$t('selection.bar.count', { values: { count: taskSelectionStore.count } })}
		</span>
		<Button
			variant="secondary"
			size="sm"
			onclick={onGroup}
			disabled={!canGroup || busy}
			class="gap-1.5"
		>
			<StackIcon class="size-4" weight="bold" />
			<span>{$t('selection.bar.group')}</span>
		</Button>
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={() => taskSelectionStore.disable()}
			aria-label={$t('selection.bar.cancel')}
			title={$t('selection.bar.cancel')}
		>
			<XIcon class="size-4" />
		</Button>
	</div>
{/if}
