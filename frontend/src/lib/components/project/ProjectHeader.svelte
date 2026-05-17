<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import ArchiveIcon from 'phosphor-svelte/lib/Archive';
	import XIcon from 'phosphor-svelte/lib/X';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import ArrowsInLineVerticalIcon from 'phosphor-svelte/lib/ArrowsInLineVertical';
	import ArrowsOutLineVerticalIcon from 'phosphor-svelte/lib/ArrowsOutLineVertical';
	import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwise';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import TriangleIcon from 'phosphor-svelte/lib/Triangle';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import BugIcon from 'phosphor-svelte/lib/Bug';
	import PencilSimpleIcon from 'phosphor-svelte/lib/PencilSimple';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import { t } from '$lib/i18n';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { troikiStore } from '$lib/stores/troiki.svelte';
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
		onTogglePrivate,
		onCreateBug,
		hasCollapsible = false,
		allSubtasksCollapsed = false,
		onToggleAllSubtasks
	}: {
		project: Project;
		hasCollapsible?: boolean;
		allSubtasksCollapsed?: boolean;
		onToggleAllSubtasks?: () => void;
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
		onCreateBug?: () => void;
	} = $props();

	const TROIKI_OPTIONS: Array<{ category: TroikiCategory; labelKey: string }> = [
		{ category: 'important', labelKey: 'troiki.section.important' },
		{ category: 'medium', labelKey: 'troiki.section.medium' },
		{ category: 'rest', labelKey: 'troiki.section.rest' }
	];

	// Load Troiki slot fills lazily: needed only when the user opens the
	// "Assign to Troika" submenu, so we trigger on first interaction with the
	// outer dropdown rather than on mount.
	let troikiLoaded = $state(false);
	let troikiLoading = $state(false);
	async function ensureTroikiLoaded(): Promise<void> {
		if (troikiLoaded || troikiLoading) return;
		troikiLoading = true;
		try {
			await troikiStore.load();
			troikiLoaded = true;
		} catch {
			// Silent — submenu will fall back to no counts and the backend will
			// reject over-cap assignments with a toast.
		} finally {
			troikiLoading = false;
		}
	}

	const troikiFills = $derived.by(() => {
		const v = troikiStore.value;
		return {
			important: { count: v.important.projects.length, cap: v.important.capacity },
			medium: { count: v.medium.projects.length, cap: v.medium.capacity },
			rest: { count: v.rest.projects.length, cap: v.rest.capacity }
		};
	});

	const STATUS_KEY: Record<Project['status'], string> = {
		open: 'project.statusOpen',
		completed: 'project.statusCompleted',
		archived: 'project.statusArchived',
		cancelled: 'project.statusCancelled'
	};

	function back(): void {
		if (history.length > 1) history.back();
		else void goto(resolve('/inbox'));
	}
</script>

<header class="flex flex-col gap-2 border-b border-border px-4 py-3 sm:px-6 sm:py-4">
	<div class="flex items-center justify-between gap-3">
		<div class="flex min-w-0 items-center gap-2">
			<Button
				variant="ghost"
				size="sm"
				onclick={back}
				aria-label={$t('common.back')}
				title={$t('common.back')}
				class="size-7 shrink-0 p-0"
			>
				<ArrowLeftIcon class="size-4" />
			</Button>
			<span
				class="inline-block size-3 shrink-0 rounded-full"
				style={`background-color: ${project.color}`}
				aria-hidden="true"
			></span>
			<h1 class="truncate text-xl font-semibold">{project.title}</h1>
			{#if project.troikiCategory}
				<span class="inline-flex" title={$t('task.inTroikiTitle')}>
					<TroikiTriggerIcon class="size-3.5 text-muted-foreground/50" />
				</span>
			{/if}
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
			{#if project.status === 'completed' || project.status === 'cancelled'}
				<Button size="sm" variant="outline" onclick={onUncomplete}>
					<ArrowCounterClockwiseIcon class="size-4" />
					{$t('project.reopen')}
				</Button>
			{/if}
			{#if hasCollapsible && onToggleAllSubtasks}
				<Button
					size="sm"
					variant="ghost"
					onclick={onToggleAllSubtasks}
					aria-label={allSubtasksCollapsed ? $t('project.expandAllSubtasks') : $t('project.collapseAllSubtasks')}
					title={allSubtasksCollapsed ? $t('project.expandAllSubtasks') : $t('project.collapseAllSubtasks')}
					class="size-7 p-0"
				>
					{#if allSubtasksCollapsed}
						<ArrowsOutLineVerticalIcon class="size-3.5 text-muted-foreground/60" />
					{:else}
						<ArrowsInLineVerticalIcon class="size-3.5 text-muted-foreground/60" />
					{/if}
				</Button>
			{/if}
			{#if project.projectType === 'software' && onCreateBug}
				<Button
					size="sm"
					variant="ghost"
					onclick={onCreateBug}
					aria-label={$t('project.createBugAriaLabel')}
					title={$t('project.createBugAriaLabel')}
				>
					<BugIcon class="size-3.5 text-muted-foreground/50" />
				</Button>
			{/if}
			<DropdownMenu.Root onOpenChange={(o) => o && void ensureTroikiLoaded()}>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button {...props} size="sm" variant="ghost" aria-label={$t('project.actionsAriaLabel')}>
							<DotsThreeIcon class="size-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="min-w-[14rem] rounded-md">
					{#if onAddSection && project.status === 'open'}
						<DropdownMenu.Item onclick={onAddSection}>
							<PlusIcon class="size-4" /> {$t('project.addSection')}
						</DropdownMenu.Item>
					{/if}
					{#if onEdit}
						<DropdownMenu.Item onclick={onEdit}>
							<PencilSimpleIcon class="size-4" /> {$t('common.edit')}
						</DropdownMenu.Item>
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
							<DropdownMenu.SubContent class="min-w-[14rem]">
								{#each TROIKI_OPTIONS as opt (opt.category)}
									{@const active = project.troikiCategory === opt.category}
									{@const fill = troikiFills[opt.category]}
									{@const full = troikiLoaded && !active && fill.count >= fill.cap}
									<DropdownMenu.Item
										disabled={full}
										onclick={() => !full && onSetTroiki(opt.category)}
									>
										{#if active}
											<CheckIcon class="size-4" weight="bold" />
										{:else}
											<span class="size-4"></span>
										{/if}
										<span class="flex-1">{$t(opt.labelKey)}</span>
										{#if troikiLoaded}
											<span class="ml-2 text-[11px] tabular-nums text-muted-foreground">
												{fill.count}/{fill.cap}
											</span>
										{/if}
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
					{#if project.status === 'open'}
						<DropdownMenu.Item onclick={onComplete}>
							<CheckIcon class="size-4" /> {$t('project.complete')}
						</DropdownMenu.Item>
					{/if}
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
