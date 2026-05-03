<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import ArchiveIcon from 'phosphor-svelte/lib/Archive';
	import XIcon from 'phosphor-svelte/lib/X';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwise';
	import type { Project } from '$lib/api/types';

	let {
		project,
		onComplete,
		onUncomplete,
		onCancel,
		onArchive,
		onUnarchive,
		onPin,
		onUnpin,
		onEdit,
		onDelete
	}: {
		project: Project;
		onComplete?: () => void;
		onUncomplete?: () => void;
		onCancel?: () => void;
		onArchive?: () => void;
		onUnarchive?: () => void;
		onPin?: () => void;
		onUnpin?: () => void;
		onEdit?: () => void;
		onDelete?: () => void;
	} = $props();
</script>

<header class="flex flex-col gap-2 border-b border-border px-4 py-3 sm:px-6 sm:py-4">
	<div class="flex items-start justify-between gap-3">
		<div class="flex min-w-0 items-center gap-2">
			<span
				class="inline-block size-3 shrink-0 rounded-full"
				style={`background-color: ${project.color}`}
				aria-hidden="true"
			></span>
			<h1 class="truncate text-xl font-semibold">{project.title}</h1>
			{#if project.status !== 'open'}
				<Badge variant="outline" class="capitalize">{project.status}</Badge>
			{/if}
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if project.status === 'completed'}
				<Button size="sm" variant="outline" onclick={onUncomplete}>
					<ArrowCounterClockwiseIcon class="size-4" />
					Reopen
				</Button>
			{/if}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button {...props} size="sm" variant="ghost" aria-label="Project actions">
							<DotsThreeIcon class="size-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					{#if project.status === 'open'}
						<DropdownMenu.Item onclick={onComplete}>
							<CheckIcon class="size-4" /> Complete
						</DropdownMenu.Item>
					{/if}
					{#if onEdit}
						<DropdownMenu.Item onclick={onEdit}>Edit</DropdownMenu.Item>
					{/if}
					{#if project.isPinned}
						<DropdownMenu.Item onclick={onUnpin}>
							<PushPinIcon class="size-4" /> Unpin
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item onclick={onPin}>
							<PushPinIcon class="size-4" /> Pin
						</DropdownMenu.Item>
					{/if}
					{#if project.status === 'archived'}
						<DropdownMenu.Item onclick={onUnarchive}>
							<ArchiveIcon class="size-4" /> Unarchive
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item onclick={onArchive}>
							<ArchiveIcon class="size-4" /> Archive
						</DropdownMenu.Item>
					{/if}
					{#if project.status === 'open'}
						<DropdownMenu.Item onclick={onCancel}>
							<XIcon class="size-4" /> Cancel
						</DropdownMenu.Item>
					{/if}
					<DropdownMenu.Separator />
					<DropdownMenu.Item variant="destructive" onclick={onDelete}>
						<TrashIcon class="size-4" /> Delete
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
	{#if project.description}
		<p class="text-sm text-muted-foreground">{project.description}</p>
	{/if}
</header>
