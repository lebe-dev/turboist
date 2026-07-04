<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import { PRIORITY_COLOR, PRIORITY_LABEL, PRIORITY_ORDER } from '$lib/utils/priority';
	import type { Priority } from '$lib/api/types';
	import { t } from '$lib/i18n';
	import XIcon from 'phosphor-svelte/lib/X';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import FolderIcon from 'phosphor-svelte/lib/FolderOpen';
	import FlagIcon from 'phosphor-svelte/lib/Flag';

	let {
		onGroup,
		onMove,
		onSetPriority,
		busy = false
	}: {
		onGroup: () => void;
		onMove: () => void;
		onSetPriority: (priority: Priority) => void;
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
		<Button variant="secondary" size="sm" onclick={onMove} disabled={busy} class="gap-1.5">
			<FolderIcon class="size-4" weight="bold" />
			<span>{$t('selection.bar.move')}</span>
		</Button>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				disabled={busy}
				class="inline-flex h-8 items-center gap-1.5 rounded-md bg-secondary px-3 text-sm font-medium text-secondary-foreground transition-colors hover:bg-secondary/80 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
				aria-label={$t('selection.bar.priority')}
			>
				<FlagIcon class="size-4" weight="bold" />
				<span>{$t('selection.bar.priority')}</span>
			</DropdownMenu.Trigger>
			<DropdownMenu.Content class="min-w-[10rem]">
				{#each PRIORITY_ORDER as p (p)}
					<DropdownMenu.Item onSelect={() => onSetPriority(p)} class="gap-2">
						<FlagIcon
							class={`size-3.5 ${PRIORITY_COLOR[p]}`}
							weight={p === 'no-priority' ? 'regular' : 'fill'}
						/>
						<span>{PRIORITY_LABEL[p]}</span>
					</DropdownMenu.Item>
				{/each}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
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
