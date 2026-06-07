<script lang="ts">
	// VisibleToPeersBadge is the F6.4 "federated, visible to N peers" task badge
	// (US-7.1 AC2). It renders a small badge on a task that belongs to a federated
	// project, showing the count of peer instances the project is shared with and a
	// tooltip naming them. The count + names come from the task's project
	// peerInstances array (resolved once at bootstrap, US-7.1 AC3) — passed in by the
	// caller — so the badge needs no extra round-trip. It renders NOTHING when there
	// are no peers (a non-federated project, or an owner project with no peers yet),
	// so the caller can drop it onto any task unconditionally.
	import { Badge } from '$lib/components/ui/badge';
	import GlobeIcon from 'phosphor-svelte/lib/Globe';
	import { t } from '$lib/i18n';
	import { peerNamesLabel } from '$lib/federation/projectSurface';
	import type { PeerInstance } from '$lib/api/types';

	let { peers }: { peers: PeerInstance[] } = $props();

	const count = $derived(peers.length);
	const names = $derived(peerNamesLabel(peers));
	// The badge is now icon-only — the "visible to N peers" label and the named
	// instance list both live in the title/aria-label so the row stays compact.
	const label = $derived(
		`${$t('federation.visibility.badge', { values: { count } })} — ${names}`
	);
</script>

{#if count > 0}
	<Badge
		variant="outline"
		class="px-1 border-sky-300/60 text-sky-700 dark:border-sky-700/50 dark:text-sky-300"
		title={label}
		aria-label={label}
		role="status"
		data-testid="visible-to-peers-badge"
	>
		<GlobeIcon class="size-3" />
	</Badge>
{/if}
