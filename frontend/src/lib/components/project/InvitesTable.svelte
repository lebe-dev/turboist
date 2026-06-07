<script lang="ts">
	import { toast } from 'svelte-sonner';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import ProhibitIcon from 'phosphor-svelte/lib/Prohibit';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import { t } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { federation as federationApi } from '$lib/api/endpoints/federation';
	import type { Invite, InviteStatus } from '$lib/api/types';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';

	let {
		projectId,
		// sessionLinks maps an invite id to the full share link captured at creation
		// time (the only moment the secret is in memory). Copy-link is offered ONLY
		// for these — a re-visited invite never re-serves its secret (US-1.3 AC4, AC5).
		sessionLinks = {},
		onChanged
	}: {
		projectId: number;
		sessionLinks?: Record<string, string>;
		onChanged?: () => void;
	} = $props();

	let invites = $state<Invite[]>([]);
	let loading = $state(false);
	let busyId = $state<string | null>(null);

	// Semantic status tones for an invite link. An "active" (still usable) link
	// reads as calm/neutral — a grey chip (light-grey on dark, dark-grey on light)
	// rather than the brand-primary red; revoked borrows the destructive red;
	// consumed/expired sit in the muted palette. The dot is an empty sibling so the
	// label text node stays isolated.
	const STATUS_TONE: Record<InviteStatus, { pill: string; dot: string }> = {
		active: {
			pill: 'border-border bg-muted text-foreground/75',
			dot: 'bg-foreground/60'
		},
		revoked: {
			pill: 'border-destructive/25 bg-destructive/10 text-destructive',
			dot: 'bg-destructive'
		},
		consumed: {
			pill: 'border-border bg-muted text-muted-foreground',
			dot: 'bg-muted-foreground/60'
		},
		expired: {
			pill: 'border-border bg-transparent text-muted-foreground',
			dot: 'bg-muted-foreground/40'
		}
	};

	async function load() {
		const client = getApiClient();
		if (!client) return;
		loading = true;
		try {
			invites = await federationApi.listInvites(client, projectId);
		} catch (err) {
			toast.error(describeError(err, $t('federation.invite.list.loadFailed')));
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		// re-load whenever the project changes
		void projectId;
		void load();
	});

	async function copyLink(inviteId: string) {
		const link = sessionLinks[inviteId];
		if (!link) return;
		try {
			await navigator.clipboard.writeText(link);
			toast.success($t('federation.invite.copied'));
		} catch {
			// clipboard may be blocked; the owner can still copy from the creation dialog
		}
	}

	async function revoke(inviteId: string) {
		const client = getApiClient();
		if (!client) return;
		busyId = inviteId;
		try {
			await federationApi.revokeInvite(client, projectId, inviteId);
			toast.success($t('federation.invite.list.revoked'));
			await load();
			onChanged?.();
		} catch (err) {
			toast.error(describeError(err, $t('federation.invite.list.revokeFailed')));
		} finally {
			busyId = null;
		}
	}

	async function remove(inviteId: string) {
		const client = getApiClient();
		if (!client) return;
		busyId = inviteId;
		try {
			await federationApi.deleteInvite(client, projectId, inviteId);
			toast.success($t('federation.invite.list.deleted'));
			await load();
			onChanged?.();
		} catch (err) {
			toast.error(describeError(err, $t('federation.invite.list.deleteFailed')));
		} finally {
			busyId = null;
		}
	}

	function fmtDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '—';
		return d.toLocaleDateString();
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading && invites.length === 0}
		<p class="text-sm text-muted-foreground">{$t('common.loading')}</p>
	{:else if invites.length === 0}
		<p class="text-sm text-muted-foreground">{$t('federation.invite.list.empty')}</p>
	{:else}
		<ul class="flex flex-col divide-y divide-border overflow-hidden rounded-lg border border-border bg-card/40">
			{#each invites as inv (inv.inviteId)}
				<li
					data-invite-row
					class="group flex flex-col gap-1.5 px-3 py-2.5 text-sm transition-colors hover:bg-muted/40"
				>
					<!-- Primary line: invite id + status + (hover-revealed) controls. -->
					<div class="flex items-center gap-2">
						<span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground" title={inv.inviteId}>
							{inv.inviteId}
						</span>
						<span
							class={[
								'inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium leading-none',
								STATUS_TONE[inv.status].pill
							]}
						>
							<span class={['size-1.5 shrink-0 rounded-full', STATUS_TONE[inv.status].dot]}></span>{$t(
								`federation.invite.status.${inv.status}`
							)}
						</span>

						<div
							class="ml-0.5 flex shrink-0 items-center gap-0.5 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
						>
							{#if sessionLinks[inv.inviteId]}
								<Button
									type="button"
									variant="ghost"
									size="sm"
									class="size-7 p-0 text-muted-foreground hover:text-foreground"
									onclick={() => copyLink(inv.inviteId)}
									title={$t('federation.invite.copy')}
									aria-label={$t('federation.invite.copy')}
								>
									<CopyIcon class="size-4" />
								</Button>
							{/if}
							{#if inv.status === 'active'}
								<Button
									type="button"
									variant="ghost"
									size="sm"
									class="size-7 p-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
									disabled={busyId === inv.inviteId}
									onclick={() => revoke(inv.inviteId)}
									aria-label={$t('federation.invite.list.revoke')}
									title={$t('federation.invite.list.revoke')}
								>
									<ProhibitIcon class="size-4" />
								</Button>
							{:else}
								<Button
									type="button"
									variant="ghost"
									size="sm"
									class="size-7 p-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
									disabled={busyId === inv.inviteId}
									onclick={() => remove(inv.inviteId)}
									aria-label={$t('common.delete')}
									title={$t('common.delete')}
								>
									<TrashIcon class="size-4" />
								</Button>
							{/if}
						</div>
					</div>

					<!-- Secondary line: permission · uses · expiry. -->
					<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
						<span>{$t(`federation.invite.permission.${inv.permissions}`)}</span>
						<span aria-hidden="true" class="text-muted-foreground/40">·</span>
						<span>
							{$t('federation.invite.list.uses', { values: { used: inv.usedCount, max: inv.maxUses } })}
						</span>
						<span aria-hidden="true" class="text-muted-foreground/40">·</span>
						<span>
							{$t('federation.invite.list.expires', { values: { date: fmtDate(inv.expiresAt) } })}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
