<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import StarIcon from 'phosphor-svelte/lib/Star';
	import StarFilledIcon from 'phosphor-svelte/lib/StarFour';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import type { Label } from '$lib/api/types';

	let {
		label,
		onEdit,
		onDelete,
		onToggleFavourite
	}: {
		label: Label;
		onEdit?: () => void;
		onDelete?: () => void;
		onToggleFavourite?: () => void;
	} = $props();
</script>

<header class="flex items-center justify-between gap-3 border-b border-border px-6 py-4">
	<div class="flex min-w-0 items-center gap-2">
		<TagIcon class="size-4 shrink-0" style={`color: ${label.color}`} />
		<h1 class="truncate text-xl font-semibold">{label.name}</h1>
		{#if label.isFavourite}
			<StarFilledIcon class="size-4 text-amber-500" aria-label="Favourite" />
		{/if}
	</div>
	<div class="flex items-center gap-2">
		{#if onToggleFavourite}
			<Button size="sm" variant="ghost" onclick={onToggleFavourite}>
				<StarIcon class="size-4" />
				{label.isFavourite ? 'Unfavourite' : 'Favourite'}
			</Button>
		{/if}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button {...props} size="sm" variant="ghost" aria-label="Label actions">
						<DotsThreeIcon class="size-4" />
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="end">
				{#if onEdit}
					<DropdownMenu.Item onclick={onEdit}>
						<PencilIcon class="size-4" /> Edit
					</DropdownMenu.Item>
				{/if}
				{#if onDelete}
					<DropdownMenu.Separator />
					<DropdownMenu.Item variant="destructive" onclick={onDelete}>
						<TrashIcon class="size-4" /> Delete
					</DropdownMenu.Item>
				{/if}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
</header>
