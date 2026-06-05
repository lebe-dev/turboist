<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import ShieldWarningIcon from 'phosphor-svelte/lib/ShieldWarning';
	import { t, locale } from '$lib/i18n';
	import { federationStore } from '$lib/stores/federation.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import type { AuditEntry, SignatureFailureAlert } from '$lib/api/types';

	// The federation audit log (Federation v1 F6.3, US-7.4): the security-relevant
	// federation events the owner can browse to investigate anomalies — signature
	// failures, replay, key changes, handshakes, revokes — newest-first. A burst of
	// signature failures from one peer surfaces a "possible attack on peer X" banner
	// (AC3). It is a pure server read; a federation-disabled instance shows nothing.
	let loading = $state(true);
	let refreshing = $state(false);

	const entries = $derived<AuditEntry[]>(federationStore.audit);
	const alerts = $derived<SignatureFailureAlert[]>(federationStore.auditAlerts);

	onMount(load);

	async function load(): Promise<void> {
		loading = true;
		try {
			await federationStore.loadAudit({ limit: 100 });
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.audit.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function refresh(): Promise<void> {
		if (refreshing) return;
		refreshing = true;
		try {
			await federationStore.loadAudit({ limit: 100 });
		} catch (err) {
			toast.error(describeError(err, $t('settings.federation.audit.loadFailed')));
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

	// kindLabel maps an audit kind to its localized label, falling back to the raw
	// kind so a future server-side kind never renders blank.
	function kindLabel(kind: string): string {
		const key = `settings.federation.audit.kind.${kind}`;
		const label = $t(key);
		return label === key ? kind : label;
	}

	function outcomeLabel(outcome: string): string {
		const key = `settings.federation.audit.outcome.${outcome}`;
		const label = $t(key);
		return label === key ? outcome : label;
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.federation.audit.heading')}</h2>
			<p class="text-xs text-muted-foreground">
				{$t('settings.federation.audit.description')}
			</p>
		</div>
		<button
			type="button"
			class="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
			onclick={refresh}
			disabled={refreshing}
		>
			{$t('settings.federation.audit.refresh')}
		</button>
	</div>

	{#if alerts.length > 0}
		<ul class="flex flex-col gap-2" data-testid="audit-alerts">
			{#each alerts as a (a.peerInstanceUrl)}
				<li
					class="flex items-start gap-3 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2.5"
				>
					<ShieldWarningIcon class="size-4 shrink-0 text-destructive" weight="fill" />
					<span class="text-sm font-medium text-destructive">
						{$t('settings.federation.audit.attackAlert', {
							values: { peer: a.peerInstanceUrl, count: a.count }
						})}
					</span>
				</li>
			{/each}
		</ul>
	{/if}

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if entries.length === 0}
		<p class="text-xs text-muted-foreground">{$t('settings.federation.audit.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2" data-testid="audit-entries">
			{#each entries as e (e.id)}
				<li
					class="flex items-start gap-3 rounded-md border bg-muted/30 px-3 py-2.5 {e.outcome ===
					'rejected'
						? 'border-destructive/30'
						: 'border-border'}"
				>
					<WarningIcon
						class="size-4 shrink-0 {e.outcome === 'rejected'
							? 'text-destructive'
							: 'text-muted-foreground'}"
					/>
					<div class="flex min-w-0 flex-1 flex-col gap-0.5">
						<span class="flex items-center gap-2 text-sm font-medium">
							<span>{kindLabel(e.kind)}</span>
							<span
								class="rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wide {e.outcome ===
								'rejected'
									? 'bg-destructive/15 text-destructive'
									: 'bg-muted text-muted-foreground'}"
							>
								{outcomeLabel(e.outcome)}
							</span>
						</span>
						{#if e.peerInstanceUrl}
							<span class="truncate text-xs text-muted-foreground">{e.peerInstanceUrl}</span>
						{/if}
						{#if e.detail}
							<span class="text-[11px] text-muted-foreground/80">{e.detail}</span>
						{/if}
						<span class="text-[11px] text-muted-foreground/80" title={e.createdAt}>
							{formatAbsolute(e.createdAt)}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
