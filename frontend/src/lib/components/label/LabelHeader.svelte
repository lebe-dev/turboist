<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import StarIcon from 'phosphor-svelte/lib/Star';
	import StarFilledIcon from 'phosphor-svelte/lib/StarFour';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import { t } from '$lib/i18n';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import type { Label } from '$lib/api/types';

	let {
		label,
		onEdit,
		onDelete,
		onToggleFavourite,
		onTogglePrivate
	}: {
		label: Label;
		onEdit?: () => void;
		onDelete?: () => void;
		onToggleFavourite?: () => void;
		onTogglePrivate?: () => void;
	} = $props();
</script>

<header class="flex items-center justify-between gap-3 border-b border-border px-6 py-4">
	<div class="flex min-w-0 items-center gap-2">
		<TagIcon class="size-4 shrink-0" style={`color: ${label.color}`} />
		<h1 class="truncate text-xl font-semibold">{label.name}</h1>
		{#if label.isFavourite}
			<StarFilledIcon class="size-4 text-amber-500" aria-label={$t('common.favourite')} />
		{/if}
		{#if label.isPrivate && !settingsStore.publicView}
			<span
				class="inline-flex"
				title={$t('common.privateTooltip')}
				aria-label={$t('common.privateMarker')}
			>
				<LockSimpleIcon class="size-3 text-muted-foreground/40" />
			</span>
		{/if}
	</div>
	<div class="flex items-center gap-2">
		{#if onToggleFavourite}
			<Button size="sm" variant="ghost" onclick={onToggleFavourite}>
				<StarIcon class="size-4" />
				{label.isFavourite ? $t('common.unfavourite') : $t('common.favourite')}
			</Button>
		{/if}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button {...props} size="sm" variant="ghost" aria-label={$t('label.actionsAriaLabel')}>
						<DotsThreeIcon class="size-4" />
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="end">
				{#if onEdit}
					<DropdownMenu.Item onclick={onEdit}>
						<PencilIcon class="size-4" /> {$t('common.edit')}
					</DropdownMenu.Item>
				{/if}
				{#if onTogglePrivate}
					<DropdownMenu.Item onclick={onTogglePrivate}>
						{#if label.isPrivate}
							<LockSimpleOpenIcon class="size-4" /> {$t('common.unmarkPrivate')}
						{:else}
							<LockSimpleIcon class="size-4" /> {$t('common.markPrivate')}
						{/if}
					</DropdownMenu.Item>
				{/if}
				{#if onDelete}
					<DropdownMenu.Separator />
					<DropdownMenu.Item variant="destructive" onclick={onDelete}>
						<TrashIcon class="size-4" /> {$t('common.delete')}
					</DropdownMenu.Item>
				{/if}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
</header>
