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
	import CheckSquareIcon from 'phosphor-svelte/lib/CheckSquare';
	import XIcon from 'phosphor-svelte/lib/X';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import ArrowsInLineVerticalIcon from 'phosphor-svelte/lib/ArrowsInLineVertical';
	import ArrowsOutLineVerticalIcon from 'phosphor-svelte/lib/ArrowsOutLineVertical';
	import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwise';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import TriangleIcon from 'phosphor-svelte/lib/Triangle';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import GlobeSimpleIcon from 'phosphor-svelte/lib/GlobeSimple';
	import CloudSlashIcon from 'phosphor-svelte/lib/CloudSlash';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import BugIcon from 'phosphor-svelte/lib/Bug';
	import PencilSimpleIcon from 'phosphor-svelte/lib/PencilSimple';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import SyncStatusBadge from '$lib/components/project/SyncStatusBadge.svelte';
	import { t } from '$lib/i18n';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { federationStore } from '$lib/stores/federation.svelte';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { isJoinedFederated, isOwnerOffline, isReadOnlyFederated } from '$lib/federation/projectSurface';
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
		onEnableFederation,
		onCreateInvite,
		onLeaveFederation,
		federationExpanded = false,
		onToggleFederation,
		hasCollapsible = false,
		allSubtasksCollapsed = false,
		onToggleAllSubtasks
	}: {
		project: Project;
		hasCollapsible?: boolean;
		allSubtasksCollapsed?: boolean;
		onToggleAllSubtasks?: () => void;
		// When provided, the "Federated" badge becomes the disclosure control for the
		// federation details block (peers + invites): it shows a chevron and toggles
		// federationExpanded. Without it the badge is a plain static marker.
		federationExpanded?: boolean;
		onToggleFederation?: () => void;
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
		onEnableFederation?: () => void;
		onCreateInvite?: () => void;
		onLeaveFederation?: () => void;
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

	// Federation surface (Federation v1 F2.4, US-2.4). A joined copy (federated,
	// not the local owner) shows its origin instance; a read-only joined copy
	// additionally locks every mutating control. The backend 403 guard
	// (federation_read_only) is authoritative — this only hides the affordances.
	const joined = $derived(isJoinedFederated(project));
	const readOnly = $derived(isReadOnlyFederated(project));
	// Owner-death read-only/queued fallback (Federation v1 F5.6a, US-6.5 AC1): when
	// the owner instance has been unreachable past the owner-timeout window, surface
	// a "pending — owner offline" badge. It is INFORMATIONAL only — editing stays
	// enabled and edits queue until the owner returns (US-6.5 AC2), so it never
	// engages the read-only lockout.
	const ownerOffline = $derived(isOwnerOffline(project));
	// A joined copy can be voluntarily left while the trust link is still intact
	// (Federation v1 F5.5, US-6.3): once it is lost (already left, or revoked) there
	// is nothing left to leave, so the action hides. A read-only joined copy can
	// still leave — leaving turns it into an editable local project (US-6.3 AC3).
	const canLeaveFederation = $derived(joined && !project.federationLost);

	// Federation sync-status indicator (Federation v1 F4.3, US-4.3): a colour-coded
	// badge reflecting the server-derived per-project status (synced / pending /
	// unreachable / key_mismatch). Hidden for non-federated projects (the store has
	// no entry for them) and for the project itself when it is not federated.
	const syncStatus = $derived(project.isFederated ? federationStore.forProject(project.id) : undefined);

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
			{#if settingsStore.troikiEnabled && project.troikiCategory}
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
			{#if project.isFederated}
				{#if onToggleFederation}
					<!--
						Federation disclosure (request: collapse the standalone "Federation"
						panel into the badge). The "Federated" badge doubles as the toggle for
						the peers/invites details block — a chevron flips when expanded.
					-->
					<button
						type="button"
						class="inline-flex h-5 items-center gap-1 rounded-sm border border-border px-2 py-0.5 text-xs font-medium text-foreground transition-colors hover:bg-muted"
						onclick={onToggleFederation}
						aria-expanded={federationExpanded}
						title={$t('federation.badgeTooltip')}
					>
						<GlobeSimpleIcon class="size-3" />
						{$t('federation.badge')}
						<CaretDownIcon
							class={[
								'size-3 text-muted-foreground transition-transform duration-200',
								federationExpanded && 'rotate-180'
							]}
						/>
					</button>
				{:else}
					<Badge variant="outline" class="gap-1" title={$t('federation.badgeTooltip')}>
						<GlobeSimpleIcon class="size-3" />
						{$t('federation.badge')}
					</Badge>
				{/if}
			{/if}
			{#if syncStatus}
				<SyncStatusBadge status={syncStatus} />
			{/if}
			{#if joined && project.originInstance}
				<Badge
					variant="outline"
					class="max-w-[14rem] truncate"
					title={$t('federation.originTooltip', { values: { url: project.originInstance } })}
				>
					{$t('federation.origin', { values: { url: project.originInstance } })}
				</Badge>
			{/if}
			{#if readOnly}
				<Badge variant="secondary" class="gap-1" title={$t('federation.readOnlyTooltip')}>
					<LockSimpleIcon class="size-3" />
					{$t('federation.readOnly')}
				</Badge>
			{/if}
			{#if ownerOffline}
				<Badge
					variant="outline"
					class="gap-1 border-amber-500/50 text-amber-600 dark:text-amber-400"
					title={$t('federation.ownerOfflineTooltip')}
				>
					<CloudSlashIcon class="size-3" />
					{$t('federation.ownerOffline')}
				</Badge>
			{/if}
			{#if project.status !== 'open'}
				<Badge variant="outline">{$t(STATUS_KEY[project.status])}</Badge>
			{/if}
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if (project.status === 'completed' || project.status === 'cancelled') && !readOnly}
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
					{#if onAddSection && project.status === 'open' && !readOnly}
						<DropdownMenu.Item onclick={onAddSection}>
							<PlusIcon class="size-4" /> {$t('project.addSection')}
						</DropdownMenu.Item>
					{/if}
					{#if onEdit && !readOnly}
						<DropdownMenu.Item onclick={onEdit}>
							<PencilSimpleIcon class="size-4" /> {$t('common.edit')}
						</DropdownMenu.Item>
					{/if}
					<DropdownMenu.Item
						onclick={() => {
							if (!taskSelectionStore.mode) taskSelectionStore.enable();
						}}
					>
						<CheckSquareIcon class="size-4" /> {$t('task.actions.select')}
					</DropdownMenu.Item>
					{#if project.isPinned}
						<DropdownMenu.Item onclick={onUnpin}>
							<PushPinIcon class="size-4" /> {$t('project.unpin')}
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item onclick={onPin}>
							<PushPinIcon class="size-4" /> {$t('project.pin')}
						</DropdownMenu.Item>
					{/if}
					{#if onTogglePrivate && !readOnly}
						<DropdownMenu.Item onclick={onTogglePrivate}>
							{#if project.isPrivate}
								<LockSimpleOpenIcon class="size-4" /> {$t('common.unmarkPrivate')}
							{:else}
								<LockSimpleIcon class="size-4" /> {$t('common.markPrivate')}
							{/if}
						</DropdownMenu.Item>
					{/if}
					{#if onEnableFederation && !project.isFederated && project.status === 'open'}
						<DropdownMenu.Item onclick={onEnableFederation}>
							<GlobeSimpleIcon class="size-4" /> {$t('federation.enable')}
						</DropdownMenu.Item>
					{/if}
					{#if onCreateInvite && project.isFederated && project.status === 'open' && !readOnly}
						<DropdownMenu.Item onclick={onCreateInvite}>
							<GlobeSimpleIcon class="size-4" /> {$t('federation.invite.action')}
						</DropdownMenu.Item>
					{/if}
					{#if onLeaveFederation && canLeaveFederation}
						<DropdownMenu.Item onclick={onLeaveFederation}>
							<SignOutIcon class="size-4" /> {$t('federation.leave.action')}
						</DropdownMenu.Item>
					{/if}
					{#if onSetTroiki && project.status === 'open' && settingsStore.troikiEnabled && !readOnly}
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
					{#if !readOnly}
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
					{/if}
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>
	{#if project.description}
		<p class="whitespace-pre-line text-sm text-muted-foreground">{project.description}</p>
	{/if}
</header>
