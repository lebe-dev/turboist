<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PauseIcon from 'phosphor-svelte/lib/Pause';
	import PlayIcon from 'phosphor-svelte/lib/Play';
	import ProhibitIcon from 'phosphor-svelte/lib/Prohibit';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import KeyIcon from 'phosphor-svelte/lib/Key';
	import { t } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { federation as federationApi } from '$lib/api/endpoints/federation';
	import type { Peer } from '$lib/api/types';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import PeerStatusBadge from '$lib/components/project/PeerStatusBadge.svelte';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import { eventsClient } from '$lib/realtime/events.svelte';

	let { projectId }: { projectId: number } = $props();

	let peers = $state<Peer[]>([]);
	let loading = $state(false);
	// busyUrl gates double-clicks while a pause/resume/revoke request is in flight
	// for a single peer (Federation v1 F5.3 / F5.4).
	let busyUrl = $state<string | null>(null);
	// revokeTarget holds the peer the owner is confirming a permanent revoke for
	// (Federation v1 F5.4, US-6.2). Revoke is irreversible, so it goes through a
	// confirm dialog; null means the dialog is closed.
	let revokeTarget = $state<Peer | null>(null);
	let revokeOpen = $state(false);
	// trustTarget holds the peer whose new key the owner is confirming a trust for
	// (Federation v1 F5.6b, US-6.4 AC3). Trusting a rotated key is a deliberate
	// security action, so it goes through a confirm dialog; null means closed.
	let trustTarget = $state<Peer | null>(null);
	let trustOpen = $state(false);

	async function load() {
		const client = getApiClient();
		if (!client) return;
		loading = true;
		try {
			peers = await federationApi.listPeers(client, projectId);
		} catch (err) {
			toast.error(describeError(err, $t('federation.peers.loadFailed')));
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		// re-load whenever the project changes
		void projectId;
		void load();
	});

	// Live delivery-overdue indicator (Federation v1 F3.2, US-3.2 AC4): a
	// federation-origin change arrives on the projects/tasks scopes (not
	// echo-suppressed), so reload the peers list — and with it pendingDelivery —
	// whenever sync activity touches this workspace.
	$effect(() => {
		const reload = () => void load();
		const unsubs = [
			eventsClient.on('projects', reload),
			eventsClient.on('tasks', reload),
			eventsClient.on('sections', reload)
		];
		return () => {
			for (const u of unsubs) u();
		};
	});

	// pause temporarily pauses exchange with a peer (Federation v1 F5.3, US-6.1
	// AC1): the owner's outbox stops fanning out to it (events accumulate) without
	// breaking the trust link. On success the list reloads so the row flips to the
	// paused status (US-6.1 AC3).
	async function pause(peer: Peer) {
		const client = getApiClient();
		if (!client) return;
		busyUrl = peer.instanceUrl;
		try {
			await federationApi.pausePeer(client, projectId, peer.instanceUrl);
			toast.success(
				$t('federation.peers.paused', { values: { name: peer.displayName || peer.instanceUrl } })
			);
			await load();
		} catch (err) {
			toast.error(describeError(err, $t('federation.peers.pauseFailed')));
		} finally {
			busyUrl = null;
		}
	}

	// resume un-pauses a peer (Federation v1 F5.3, US-6.1 AC2): the owner's outbox
	// resumes and flushes the accumulated events.
	async function resume(peer: Peer) {
		const client = getApiClient();
		if (!client) return;
		busyUrl = peer.instanceUrl;
		try {
			await federationApi.resumePeer(client, projectId, peer.instanceUrl);
			toast.success(
				$t('federation.peers.resumed', { values: { name: peer.displayName || peer.instanceUrl } })
			);
			await load();
		} catch (err) {
			toast.error(describeError(err, $t('federation.peers.resumeFailed')));
		} finally {
			busyUrl = null;
		}
	}

	// askRevoke opens the irreversible-revoke confirm dialog for a peer (Federation
	// v1 F5.4, US-6.2). The actual DELETE only fires on confirm (confirmRevoke).
	function askRevoke(peer: Peer) {
		revokeTarget = peer;
		revokeOpen = true;
	}

	// confirmRevoke permanently revokes the confirmed peer (Federation v1 F5.4,
	// US-6.2 AC1): the owner DELETEs the peer, which flips revoked, sends the peer a
	// federation_revoke event (so it self-marks read-only), and halts. Irreversible —
	// re-collaboration needs a fresh invite. On success the list reloads so the row
	// flips to the revoked status with no further controls.
	async function confirmRevoke() {
		const peer = revokeTarget;
		if (!peer) return;
		const client = getApiClient();
		if (!client) return;
		busyUrl = peer.instanceUrl;
		try {
			await federationApi.revokePeer(client, projectId, peer.instanceUrl);
			toast.success(
				$t('federation.peers.revoked', { values: { name: peer.displayName || peer.instanceUrl } })
			);
			await load();
		} catch (err) {
			toast.error(describeError(err, $t('federation.peers.revokeFailed')));
		} finally {
			busyUrl = null;
			revokeTarget = null;
		}
	}

	// askTrust opens the trust-new-key confirm dialog for a peer whose signature
	// stopped validating (Federation v1 F5.6b, US-6.4 AC2/AC3). The actual POST only
	// fires on confirm (confirmTrust) — trusting a rotated key is a security action.
	function askTrust(peer: Peer) {
		trustTarget = peer;
		trustOpen = true;
	}

	// confirmTrust manually trusts the confirmed peer's new key (Federation v1 F5.6b,
	// US-6.4 AC3): the server fetches the peer's current .well-known key, overwrites
	// the pinned key, clears the incident, and resumes applying the peer's updates. On
	// success the list reloads so the incident alert clears.
	async function confirmTrust() {
		const peer = trustTarget;
		if (!peer) return;
		const client = getApiClient();
		if (!client) return;
		busyUrl = peer.instanceUrl;
		try {
			await federationApi.trustKey(client, projectId, peer.instanceUrl);
			toast.success(
				$t('federation.peers.trustKeyDone', { values: { name: peer.displayName || peer.instanceUrl } })
			);
			await load();
		} catch (err) {
			toast.error(describeError(err, $t('federation.peers.trustKeyFailed')));
		} finally {
			busyUrl = null;
			trustTarget = null;
		}
	}

	function fmtDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '—';
		return d.toLocaleString();
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading && peers.length === 0}
		<p class="text-sm text-muted-foreground">{$t('common.loading')}</p>
	{:else if peers.length === 0}
		<p class="text-sm text-muted-foreground">{$t('federation.peers.empty')}</p>
	{:else}
		<ul class="flex flex-col divide-y divide-border overflow-hidden rounded-lg border border-border bg-card/40">
			{#each peers as peer (peer.instanceUrl)}
				{@const terminal = peer.status === 'revoked' || peer.status === 'left'}
				<li
					data-peer-row
					class="group flex flex-col gap-1.5 px-3 py-2.5 text-sm transition-colors hover:bg-muted/40"
				>
					<!-- Primary line: who + link health + (hover-revealed) controls. -->
					<div class="flex items-center gap-2">
						<span class="min-w-0 flex-1 truncate font-medium text-foreground" title={peer.instanceUrl}>
							{$t('federation.peers.identity', {
								values: { name: peer.displayName || peer.instanceUrl, url: peer.instanceUrl }
							})}
						</span>
						<PeerStatusBadge status={peer.status} />

						<!--
							Action cluster. Quiet at rest (revealed on hover/focus on pointer
							devices, always visible on touch) so the row reads as status, not a
							toolbar. Pause/resume (F5.3, US-6.1), trust-new-key (F5.6b, US-6.4),
							and revoke (F5.4, US-6.2) — none shown for a terminal peer (revoked /
							left). Revoke goes red only on hover; trust-key is amber to match its
							incident alert.
						-->
						{#if !terminal || peer.status === 'paused'}
							<div
								class="ml-0.5 flex shrink-0 items-center gap-0.5 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
							>
								{#if peer.status === 'paused'}
									<Button
										type="button"
										variant="ghost"
										size="sm"
										class="size-7 p-0 text-muted-foreground hover:text-foreground"
										disabled={busyUrl === peer.instanceUrl}
										onclick={() => resume(peer)}
										aria-label={$t('federation.peers.resume')}
										title={$t('federation.peers.resume')}
									>
										<PlayIcon class="size-4" />
									</Button>
								{:else if !terminal}
									<Button
										type="button"
										variant="ghost"
										size="sm"
										class="size-7 p-0 text-muted-foreground hover:text-foreground"
										disabled={busyUrl === peer.instanceUrl}
										onclick={() => pause(peer)}
										aria-label={$t('federation.peers.pause')}
										title={$t('federation.peers.pause')}
									>
										<PauseIcon class="size-4" />
									</Button>
								{/if}

								{#if peer.keyMismatchAt && !terminal}
									<Button
										type="button"
										variant="ghost"
										size="sm"
										class="size-7 p-0 text-amber-600 hover:bg-amber-500/10 hover:text-amber-700 dark:text-amber-400"
										disabled={busyUrl === peer.instanceUrl}
										onclick={() => askTrust(peer)}
										aria-label={$t('federation.peers.trustKey')}
										title={$t('federation.peers.trustKey')}
									>
										<KeyIcon class="size-4" />
									</Button>
								{/if}

								{#if !terminal}
									<Button
										type="button"
										variant="ghost"
										size="sm"
										class="size-7 p-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
										disabled={busyUrl === peer.instanceUrl}
										onclick={() => askRevoke(peer)}
										aria-label={$t('federation.peers.revoke')}
										title={$t('federation.peers.revoke')}
									>
										<ProhibitIcon class="size-4" />
									</Button>
								{/if}
							</div>
						{/if}
					</div>

					<!-- Secondary line: permission · last contact · delivery backlog. -->
					<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
						<span>{$t(`federation.invite.permission.${peer.permissions}`)}</span>
						<span aria-hidden="true" class="text-muted-foreground/40">·</span>
						<span>{$t('federation.peers.lastContact', { values: { date: fmtDate(peer.lastContactAt) } })}</span>
						<span aria-hidden="true" class="text-muted-foreground/40">·</span>
						<span class={peer.pendingDelivery > 0 ? 'font-medium text-amber-600 dark:text-amber-400' : ''}>
							{$t('federation.peers.pending', { values: { count: peer.pendingDelivery } })}
						</span>
					</div>

					<!--
						Key-change incident alert (Federation v1 F5.6b, US-6.4 AC2). The peer's
						signature failed — a possible key rotation or compromise — and its
						updates are rejected until the owner trusts the new key. Rendered as a
						distinct amber callout so the warning is unmistakable.
					-->
					{#if peer.keyMismatchAt}
						<p
							data-key-incident
							class="flex items-start gap-1.5 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1.5 text-xs font-medium text-amber-700 dark:text-amber-300"
						>
							<WarningIcon class="mt-0.5 size-3.5 shrink-0" />
							<span>
								{$t('federation.peers.keyIncident', {
									values: { name: peer.displayName || peer.instanceUrl }
								})}
							</span>
						</p>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<!--
	Irreversible-revoke confirmation (Federation v1 F5.4, US-6.2 AC5): revoke cannot
	be undone, so the owner confirms before the DELETE fires.
-->
<ConfirmDestructiveDialog
	bind:open={revokeOpen}
	title={$t('federation.peers.revokeConfirmTitle')}
	description={$t('federation.peers.revokeConfirmBody', {
		values: { name: revokeTarget?.displayName || revokeTarget?.instanceUrl || '' }
	})}
	confirmLabel={$t('federation.peers.revoke')}
	onConfirm={confirmRevoke}
/>

<!--
	Trust-new-key confirmation (Federation v1 F5.6b, US-6.4 AC3): manually trusting a
	rotated key is a security-relevant action, so the owner confirms — and is warned
	that trusting an attacker's key would accept forged updates — before the POST fires.
-->
<ConfirmDestructiveDialog
	bind:open={trustOpen}
	title={$t('federation.peers.trustKeyConfirmTitle')}
	description={$t('federation.peers.trustKeyConfirmBody', {
		values: { name: trustTarget?.displayName || trustTarget?.instanceUrl || '' }
	})}
	confirmLabel={$t('federation.peers.trustKey')}
	onConfirm={confirmTrust}
/>
