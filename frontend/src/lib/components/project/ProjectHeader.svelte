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
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import TriangleIcon from 'phosphor-svelte/lib/Triangle';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import { t } from '$lib/i18n';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import type { Project, TroikiCategory } from '$lib/api/types';

	let {
		project,
		onAddSection,
		onComplete,
		onUncomplete,
		onCancel,
		onArchive,
		onUnarchive,
		onPin,
		onUnpin,
		onEdit,
		onDelete,
		onSetTroiki,
		onTogglePrivate
	}: {
		project: Project;
		onAddSection?: () => void;
		onComplete?: () => void;
		onUncomplete?: () => void;
		onCancel?: () => void;
		onArchive?: () => void;
		onUnarchive?: () => void;
		onPin?: () => void;
		onUnpin?: () => void;
		onEdit?: () => void;
		onDelete?: () => void;
		onSetTroiki?: (category: TroikiCategory | null) => void;
		onTogglePrivate?: () => void;
	} = $props();

	const TROIKI_OPTIONS: Array<{ category: TroikiCategory; labelKey: string }> = [
		{ category: 'important', labelKey: 'troiki.section.important' },
		{ category: 'medium', labelKey: 'troiki.section.medium' },
		{ category: 'rest', labelKey: 'troiki.section.rest' }
	];

	const STATUS_KEY: Record<Project['status'], string> = {
		open: 'project.statusOpen',
		completed: 'project.statusCompleted',
		archived: 'project.statusArchived',
		cancelled: 'project.statusCancelled'
	};
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
			{#if project.isPrivate && !settingsStore.publicView}
				<span
					class="inline-flex"
					title={$t('common.privateTooltip')}
					aria-label={$t('common.privateMarker')}
				>
					<LockSimpleIcon class="size-3 text-muted-foreground/40" />
				</span>
			{/if}
			{#if project.status !== 'open'}
				<Badge variant="outline">{$t(STATUS_KEY[project.status])}</Badge>
			{/if}
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if project.status === 'completed'}
				<Button size="sm" variant="outline" onclick={onUncomplete}>
					<ArrowCounterClockwiseIcon class="size-4" />
					{$t('project.reopen')}
				</Button>
			{/if}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button {...props} size="sm" variant="ghost" aria-label={$t('project.actionsAriaLabel')}>
							<DotsThreeIcon class="size-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					{#if project.status === 'open'}
						<DropdownMenu.Item onclick={onComplete}>
							<CheckIcon class="size-4" /> {$t('project.complete')}
						</DropdownMenu.Item>
					{/if}
					{#if onAddSection && project.status === 'open'}
						<DropdownMenu.Item onclick={onAddSection}>
							<PlusIcon class="size-4" /> {$t('project.addSection')}
						</DropdownMenu.Item>
					{/if}
					{#if onEdit}
						<DropdownMenu.Item onclick={onEdit}>{$t('common.edit')}</DropdownMenu.Item>
					{/if}
					{#if project.isPinned}
						<DropdownMenu.Item onclick={onUnpin}>
							<PushPinIcon class="size-4" /> {$t('project.unpin')}
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item onclick={onPin}>
							<PushPinIcon class="size-4" /> {$t('project.pin')}
						</DropdownMenu.Item>
					{/if}
					{#if onTogglePrivate}
						<DropdownMenu.Item onclick={onTogglePrivate}>
							{#if project.isPrivate}
								<LockSimpleOpenIcon class="size-4" /> {$t('common.unmarkPrivate')}
							{:else}
								<LockSimpleIcon class="size-4" /> {$t('common.markPrivate')}
							{/if}
						</DropdownMenu.Item>
					{/if}
					{#if onSetTroiki && project.status === 'open'}
						<DropdownMenu.Sub>
							<DropdownMenu.SubTrigger>
								<TriangleIcon class="size-4" /> {$t('project.assignToTroiki')}
							</DropdownMenu.SubTrigger>
							<DropdownMenu.SubContent class="min-w-[12rem]">
								{#each TROIKI_OPTIONS as opt (opt.category)}
									{@const active = project.troikiCategory === opt.category}
									<DropdownMenu.Item onclick={() => onSetTroiki(opt.category)}>
										{#if active}
											<CheckIcon class="size-4" weight="bold" />
										{:else}
											<span class="size-4"></span>
										{/if}
										{$t(opt.labelKey)}
									</DropdownMenu.Item>
								{/each}
								{#if project.troikiCategory !== null}
									<DropdownMenu.Separator />
									<DropdownMenu.Item onclick={() => onSetTroiki(null)}>
										<XIcon class="size-4" /> {$t('project.removeFromTroiki')}
									</DropdownMenu.Item>
								{/if}
							</DropdownMenu.SubContent>
						</DropdownMenu.Sub>
					{/if}
					<DropdownMenu.Separator />
					{#if project.status === 'archived'}
						<DropdownMenu.Item onclick={onUnarchive}>
							<ArchiveIcon class="size-4" /> {$t('project.unarchive')}
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item onclick={onArchive}>
							<ArchiveIcon class="size-4" /> {$t('project.archive')}
						</DropdownMenu.Item>
					{/if}
					{#if project.status === 'open'}
						<DropdownMenu.Item onclick={onCancel}>
							<XIcon class="size-4" /> {$t('project.cancel')}
						</DropdownMenu.Item>
					{/if}
					<DropdownMenu.Separator />
					<DropdownMenu.Item variant="destructive" onclick={onDelete}>
						<TrashIcon class="size-4" /> {$t('common.delete')}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
	{#if project.description}
		<p class="text-sm text-muted-foreground">{project.description}</p>
	{/if}
</header>
