<script lang="ts">
	import WifiSlashIcon from 'phosphor-svelte/lib/WifiSlash';
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import { t } from '$lib/i18n';
	import { statusStore } from '$lib/offline/status.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import type { EventScope } from '$lib/realtime/events.svelte';

	// Page-level views revalidate off the same channel SSE uses.
	const INVALIDATE_SCOPES: EventScope[] = [
		'tasks',
		'calendar',
		'inbox',
		'projects',
		'labels',
		'contexts',
		'sections',
		'plan'
	];

	let busy = $state(false);

	// "as of {at}" reads best as a wall-clock time; the raw ISO wire value is not
	// user-facing. Falls back to the raw string if the timestamp is unparseable.
	function formatSyncTime(at: string): string {
		const d = new Date(at);
		return Number.isNaN(d.getTime()) ? at : d.toLocaleString();
	}

	async function retry(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			// A single round-trip that fans out to every shell store; each request
			// runs through ApiClient, so its outcome updates the online/offline
			// heuristic and — on success — the banner hides reactively.
			await configStore.load();
		} catch {
			// Still offline: the failed request already recorded the outcome, so the
			// banner stays. Nothing else to do here.
		} finally {
			busy = false;
		}
		// Nudge the active list view to refetch now that we may be back online.
		for (const scope of INVALIDATE_SCOPES) {
			window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope } }));
		}
	}
</script>

{#if !statusStore.online}
	<div
		class="flex items-center gap-3 border-b border-amber-500/30 bg-amber-500/10 py-2 pl-3 pr-4 text-sm text-foreground sm:pl-4 sm:pr-6"
		role="alert"
	>
		<WifiSlashIcon class="size-4 shrink-0 text-amber-600 dark:text-amber-400" weight="fill" />
		<span class="min-w-0 flex-1">
			{#if statusStore.lastSyncAt}
				{$t('offline.bannerStale', { values: { at: formatSyncTime(statusStore.lastSyncAt) } })}
			{:else}
				{$t('offline.banner')}
			{/if}
		</span>
		<button
			type="button"
			onclick={retry}
			disabled={busy}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-amber-500/40 bg-background/60 px-2.5 py-1 text-xs font-medium transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
		>
			<ArrowsClockwiseIcon class="size-3.5 {busy ? 'animate-spin' : ''}" />
			{$t('offline.retry')}
		</button>
	</div>
{/if}
