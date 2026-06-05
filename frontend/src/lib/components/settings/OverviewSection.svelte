<script lang="ts">
	// The privacy / federation overview (Federation v1 F6.4, US-7.1 AC1): a table of
	// every federated project, this instance's role (owner / peer / read-only), and
	// the named peer list it is visible to. It is a pure server read; a federation-
	// disabled instance shows nothing. So there are no privacy surprises — the owner
	// can see at a glance which projects leave this instance and to whom.
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import GlobeIcon from 'phosphor-svelte/lib/Globe';
	import { t } from '$lib/i18n';
	import { federationStore } from '$lib/stores/federation.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import type { OverviewProject } from '$lib/api/types';

	let loading = $state(true);
	let refreshing = $state(false);

	const projects = $derived<OverviewProject[]>(federationStore.overview);

	onMount(load);

	async function load(): Promise<void> {
		loading = true;
		try {
			await federationStore.loadOverview();
		} catch (err) {
			toast.error(describeError(err, $t('federation.overview.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function refresh(): Promise<void> {
		if (refreshing) return;
		refreshing = true;
		try {
			await federationStore.loadOverview();
		} catch (err) {
			toast.error(describeError(err, $t('federation.overview.loadFailed')));
		} finally {
			refreshing = false;
		}
	}

	// roleLabel maps a role to its localized label, falling back to the raw role so
	// a future server-side role never renders blank.
	function roleLabel(role: string): string {
		const key = `federation.overview.role.${role}`;
		const label = $t(key);
		return label === key ? role : label;
	}

	function peerNames(p: OverviewProject): string {
		return p.peers.map((peer) => peer.displayName || peer.instanceUrl).join(', ');
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('federation.overview.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('federation.overview.description')}</p>
		</div>
		<button
			type="button"
			class="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
			onclick={refresh}
			disabled={refreshing}
		>
			{$t('federation.overview.refresh')}
		</button>
	</div>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if projects.length === 0}
		<p class="text-xs text-muted-foreground">{$t('federation.overview.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2" data-testid="overview-projects">
			{#each projects as p (p.projectId)}
				<li class="flex items-start gap-3 rounded-md border border-border bg-muted/30 px-3 py-2.5">
					<GlobeIcon class="size-4 shrink-0 text-muted-foreground" />
					<div class="flex min-w-0 flex-1 flex-col gap-0.5">
						<span class="flex items-center gap-2 text-sm font-medium">
							<span class="truncate">{p.title}</span>
							<span
								class="rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground"
							>
								{roleLabel(p.role)}
							</span>
						</span>
						{#if p.peers.length > 0}
							<span class="text-xs text-muted-foreground">
								{$t('federation.overview.columnPeers')}: {peerNames(p)}
							</span>
						{:else}
							<span class="text-xs text-muted-foreground/70">
								{$t('federation.overview.noPeers')}
							</span>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
