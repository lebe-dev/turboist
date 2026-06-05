<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import WarningOctagonIcon from 'phosphor-svelte/lib/WarningOctagon';
	import { t, locale } from '$lib/i18n';
	import { federationStore } from '$lib/stores/federation.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import type { DeadLetterEntry } from '$lib/api/types';

	// The dead-letter diagnostics view (Federation v1 F4.4, US-4.4 AC3): the
	// parked, permanently-failed outbound events the worker did not retry. It is a
	// pure server read (no client outbox); a federation-disabled instance simply
	// surfaces an empty list. Refreshable so the owner can re-check after a fix.
	let loading = $state(true);
	let refreshing = $state(false);

	const entries = $derived<DeadLetterEntry[]>(federationStore.deadLetter);

	onMount(load);

	async function load(): Promise<void> {
		loading = true;
		try {
			await federationStore.loadDeadLetter();
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.deadLetter.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function refresh(): Promise<void> {
		if (refreshing) return;
		refreshing = true;
		try {
			await federationStore.loadDeadLetter();
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.deadLetter.loadFailed')));
		} finally {
			refreshing = false;
		}
	}

	function formatAbsolute(iso: string): string {
		try {
			return new Date(iso).toLocaleString($locale || 'en');
		} catch {
			return iso;
		}
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.federation.deadLetter.heading')}</h2>
			<p class="text-xs text-muted-foreground">
				{$t('settings.federation.deadLetter.description')}
			</p>
		</div>
		<button
			type="button"
			class="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
			onclick={refresh}
			disabled={refreshing}
		>
			{$t('settings.federation.deadLetter.refresh')}
		</button>
	</div>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if entries.length === 0}
		<p class="text-xs text-muted-foreground">{$t('settings.federation.deadLetter.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each entries as e (e.peerInstanceUrl + ':' + e.eventId)}
				<li
					class="flex items-start gap-3 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2.5"
				>
					<WarningOctagonIcon class="size-4 shrink-0 text-destructive" />
					<div class="flex min-w-0 flex-1 flex-col gap-0.5">
						<span class="truncate text-sm font-medium">{e.peerInstanceUrl}</span>
						<span class="text-xs text-muted-foreground">
							{$t('settings.federation.deadLetter.reason')}:
							<span class="font-mono">{e.reason || e.statusCode}</span>
							{#if e.statusCode}<span class="text-muted-foreground/70"> ({e.statusCode})</span>{/if}
						</span>
						<span class="font-mono text-[11px] text-muted-foreground/80">
							{$t('settings.federation.deadLetter.eventId')}: {e.eventId}
						</span>
						<span class="text-[11px] text-muted-foreground/80" title={e.failedAt}>
							{$t('settings.federation.deadLetter.failedAt')}: {formatAbsolute(e.failedAt)}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
