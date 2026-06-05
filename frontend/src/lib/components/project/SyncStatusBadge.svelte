<script lang="ts">
	// SyncStatusBadge is the F4.3 per-project federation sync-status indicator
	// (US-4.3). It renders a colour-coded badge on a federated project's header
	// reflecting the server-derived status: synced (green), pending (yellow, "N
	// changes pending"), unreachable (orange, "peer X unreachable"), key_mismatch
	// (red, "peer X key mismatch — manual action needed"). The status is a pure
	// server read (there is no client outbox) sourced from federationStore. It
	// renders NOTHING when there is no status (a non-federated project), so the
	// caller can drop it onto any project header unconditionally.
	import { Badge } from '$lib/components/ui/badge';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import ClockIcon from 'phosphor-svelte/lib/Clock';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import WarningOctagonIcon from 'phosphor-svelte/lib/WarningOctagon';
	import { t } from '$lib/i18n';
	import type { SyncStatus } from '$lib/api/types';

	let { status }: { status: SyncStatus | undefined } = $props();

	// label is the visible badge text; title is the longer tooltip (naming the
	// offending peer / pending count) the badge exposes on hover.
	const label = $derived.by(() => {
		if (!status) return '';
		switch (status.status) {
			case 'synced':
				return $t('federation.sync.synced');
			case 'pending':
				return $t('federation.sync.pending', { values: { count: status.pendingCount } });
			case 'unreachable':
				return $t('federation.sync.unreachable');
			case 'key_mismatch':
				return $t('federation.sync.keyMismatch');
			default:
				return '';
		}
	});

	const title = $derived.by(() => {
		if (!status) return '';
		switch (status.status) {
			case 'synced':
				return $t('federation.sync.syncedTooltip');
			case 'pending':
				return $t('federation.sync.pendingTooltip', { values: { count: status.pendingCount } });
			case 'unreachable':
				return $t('federation.sync.unreachableTooltip', {
					values: { peer: status.unreachablePeer }
				});
			case 'key_mismatch':
				return $t('federation.sync.keyMismatchTooltip', {
					values: { peer: status.keyMismatchPeer }
				});
			default:
				return '';
		}
	});

	// Per-state colour classes (matching the ResyncBanner amber palette idiom):
	// green / amber / orange / red, each with a dark-mode variant.
	const tone = $derived.by(() => {
		switch (status?.status) {
			case 'synced':
				return 'border-emerald-300/60 text-emerald-700 dark:border-emerald-700/50 dark:text-emerald-300';
			case 'pending':
				return 'border-amber-300/60 text-amber-800 dark:border-amber-700/50 dark:text-amber-300';
			case 'unreachable':
				return 'border-orange-300/60 text-orange-800 dark:border-orange-700/50 dark:text-orange-300';
			case 'key_mismatch':
				return 'border-red-300/60 text-red-700 dark:border-red-700/50 dark:text-red-400';
			default:
				return '';
		}
	});
</script>

{#if status}
	<Badge variant="outline" class={['gap-1', tone]} {title} role="status">
		{#if status.status === 'synced'}
			<CheckCircleIcon class="size-3" />
		{:else if status.status === 'pending'}
			<ClockIcon class="size-3" />
		{:else if status.status === 'unreachable'}
			<WarningIcon class="size-3" />
		{:else}
			<WarningOctagonIcon class="size-3" />
		{/if}
		{label}
	</Badge>
{/if}
