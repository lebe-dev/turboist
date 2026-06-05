<script lang="ts">
	// Federation ops panel (Federation v1 F6.5): the liveness/health report (US-8.1),
	// the runtime-reloadable GC retention windows (US-8.4), and the federation-aware
	// VACUUM INTO backup download (US-8.5). It is a pure owner-facing admin view; a
	// federation-disabled instance shows nothing (the loads error and are swallowed).
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import HeartbeatIcon from 'phosphor-svelte/lib/Heartbeat';
	import DownloadIcon from 'phosphor-svelte/lib/DownloadSimple';
	import { t } from '$lib/i18n';
	import { backup } from '$lib/api';
	import { federationStore } from '$lib/stores/federation.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { FederationHealth, RetentionSettings } from '$lib/api/types';

	let loading = $state(true);
	let saving = $state(false);
	let downloading = $state(false);

	const health = $derived<FederationHealth | null>(federationStore.health);
	const retention = $derived<RetentionSettings | null>(federationStore.retention);

	// Editable retention fields, seeded from the loaded settings.
	let tombstoneDays = $state(0);
	let outboxDays = $state(0);
	let inboxDays = $state(0);

	onMount(load);

	async function load(): Promise<void> {
		loading = true;
		try {
			await Promise.all([federationStore.loadHealth(), federationStore.loadRetention()]);
			seedFields();
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.ops.loadFailed')));
		} finally {
			loading = false;
		}
	}

	function seedFields(): void {
		const r = federationStore.retention;
		if (!r) return;
		tombstoneDays = r.tombstoneDays;
		outboxDays = r.outboxDays;
		inboxDays = r.inboxDays;
	}

	// statusLabel maps the health status to its localized label.
	function statusLabel(status: string): string {
		const key = `settings.federation.ops.status.${status}`;
		const label = $t(key);
		return label === key ? status : label;
	}

	// statusClass maps the health status to its badge colour classes.
	function statusClass(status: string): string {
		if (status === 'ok') return 'bg-emerald-500/15 text-emerald-600';
		if (status === 'degraded') return 'bg-amber-500/15 text-amber-600';
		if (status === 'peers_stale') return 'bg-orange-500/15 text-orange-600';
		return 'bg-muted text-muted-foreground';
	}

	// outboxOverCap reports whether the entered outbox window exceeds the §16.3
	// hardcap so the UI warns the effective value will be clamped (US-8.4).
	const outboxOverCap = $derived(retention != null && outboxDays > retention.outboxHardcapDays);

	async function saveRetention(): Promise<void> {
		if (saving) return;
		saving = true;
		try {
			await federationStore.updateRetention({
				tombstoneDays: Math.max(0, Math.trunc(tombstoneDays)),
				outboxDays: Math.max(0, Math.trunc(outboxDays)),
				inboxDays: Math.max(0, Math.trunc(inboxDays))
			});
			seedFields();
			toast.success($t('settings.federation.ops.retention.saved'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.ops.retention.saveFailed')));
		} finally {
			saving = false;
		}
	}

	async function downloadBackup(): Promise<void> {
		if (downloading) return;
		downloading = true;
		try {
			const { blob, filename } = await backup.downloadFederation();
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = filename;
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(url);
			toast.success($t('settings.federation.ops.backup.done'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.ops.backup.failed')));
		} finally {
			downloading = false;
		}
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start gap-2">
		<HeartbeatIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground" />
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.federation.ops.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.federation.ops.description')}</p>
		</div>
	</div>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else}
		<!-- Health (US-8.1) -->
		{#if health}
			<div class="flex flex-col gap-2 rounded-md border border-border bg-muted/30 px-3 py-2.5" data-testid="ops-health">
				<div class="flex items-center justify-between gap-2">
					<span class="text-xs font-medium text-muted-foreground">{$t('settings.federation.ops.health.heading')}</span>
					<span
						class="rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wide {statusClass(health.status)}"
						data-testid="ops-health-status"
					>
						{statusLabel(health.status)}
					</span>
				</div>
				<dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
					<dt class="text-muted-foreground">{$t('settings.federation.ops.health.outboxDepth')}</dt>
					<dd>{health.outboxDepth}</dd>
					<dt class="text-muted-foreground">{$t('settings.federation.ops.health.uptime')}</dt>
					<dd>{health.uptimeS}s</dd>
					<dt class="text-muted-foreground">{$t('settings.federation.ops.health.protocols')}</dt>
					<dd>{health.protocolVersions.join(', ')}</dd>
				</dl>
			</div>
		{/if}

		<!-- Retention (US-8.4) -->
		<div class="flex flex-col gap-3 rounded-md border border-border bg-muted/30 px-3 py-2.5">
			<span class="text-xs font-medium text-muted-foreground">{$t('settings.federation.ops.retention.heading')}</span>
			<p class="text-xs text-muted-foreground">{$t('settings.federation.ops.retention.hint')}</p>
			<div class="flex flex-col gap-2">
				<label class="flex items-center justify-between gap-3 text-xs">
					<span>{$t('settings.federation.ops.retention.tombstoneDays')}</span>
					<Input type="number" min="0" bind:value={tombstoneDays} class="h-7 w-24 text-xs" />
				</label>
				<label class="flex items-center justify-between gap-3 text-xs">
					<span>{$t('settings.federation.ops.retention.outboxDays')}</span>
					<Input type="number" min="0" bind:value={outboxDays} class="h-7 w-24 text-xs" />
				</label>
				<label class="flex items-center justify-between gap-3 text-xs">
					<span>{$t('settings.federation.ops.retention.inboxDays')}</span>
					<Input type="number" min="0" bind:value={inboxDays} class="h-7 w-24 text-xs" />
				</label>
			</div>
			{#if outboxOverCap && retention}
				<p class="text-xs text-amber-600" data-testid="ops-outbox-cap-warning">
					{$t('settings.federation.ops.retention.outboxCapWarning', {
						values: { cap: retention.outboxHardcapDays }
					})}
				</p>
			{/if}
			{#if retention}
				<p class="text-[11px] text-muted-foreground/80">
					{$t('settings.federation.ops.retention.effective', {
						values: {
							tombstone: retention.effectiveTombstoneDays,
							outbox: retention.effectiveOutboxDays,
							inbox: retention.effectiveInboxDays
						}
					})}
				</p>
			{/if}
			<div>
				<Button type="button" variant="secondary" size="sm" onclick={saveRetention} disabled={saving}>
					{$t('settings.federation.ops.retention.save')}
				</Button>
			</div>
		</div>

		<!-- Federation-aware backup (US-8.5) -->
		<div class="flex flex-col gap-2 rounded-md border border-border bg-muted/30 px-3 py-2.5">
			<span class="text-xs font-medium text-muted-foreground">{$t('settings.federation.ops.backup.heading')}</span>
			<p class="text-xs text-muted-foreground">{$t('settings.federation.ops.backup.hint')}</p>
			<div>
				<Button type="button" variant="secondary" size="sm" onclick={downloadBackup} disabled={downloading}>
					<DownloadIcon class="mr-1.5 size-4" />
					{$t('settings.federation.ops.backup.download')}
				</Button>
			</div>
		</div>
	{/if}
</section>
