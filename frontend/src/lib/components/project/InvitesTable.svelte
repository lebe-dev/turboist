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
	import { Badge, type BadgeVariant } from '$lib/components/ui/badge';

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

	const STATUS_VARIANT: Record<InviteStatus, BadgeVariant> = {
		active: 'default',
		revoked: 'destructive',
		consumed: 'secondary',
		expired: 'outline'
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
		<ul class="flex flex-col divide-y divide-border rounded-md border border-border">
			{#each invites as inv (inv.inviteId)}
				<li data-invite-row class="flex flex-wrap items-center gap-2 px-3 py-2 text-sm">
					<span class="font-mono text-xs text-muted-foreground" title={inv.inviteId}>
						{inv.inviteId}
					</span>
					<Badge variant={STATUS_VARIANT[inv.status]}>
						{$t(`federation.invite.status.${inv.status}`)}
					</Badge>
					<span class="text-xs text-muted-foreground">
						{$t(`federation.invite.permission.${inv.permissions}`)}
					</span>
					<span class="text-xs text-muted-foreground">
						{$t('federation.invite.list.uses', {
							values: { used: inv.usedCount, max: inv.maxUses }
						})}
					</span>
					<span class="text-xs text-muted-foreground">
						{$t('federation.invite.list.expires', { values: { date: fmtDate(inv.expiresAt) } })}
					</span>

					<div class="ml-auto flex items-center gap-1">
						{#if sessionLinks[inv.inviteId]}
							<Button
								type="button"
								variant="ghost"
								size="sm"
								onclick={() => copyLink(inv.inviteId)}
								title={$t('federation.invite.copy')}
								aria-label={$t('federation.invite.copy')}
							>
								<CopyIcon class="size-4" />
								{$t('federation.invite.copy')}
							</Button>
						{/if}
						{#if inv.status === 'active'}
							<Button
								type="button"
								variant="ghost"
								size="sm"
								disabled={busyId === inv.inviteId}
								onclick={() => revoke(inv.inviteId)}
								aria-label={$t('federation.invite.list.revoke')}
							>
								<ProhibitIcon class="size-4" />
								{$t('federation.invite.list.revoke')}
							</Button>
						{:else}
							<Button
								type="button"
								variant="ghost"
								size="sm"
								disabled={busyId === inv.inviteId}
								onclick={() => remove(inv.inviteId)}
								aria-label={$t('common.delete')}
							>
								<TrashIcon class="size-4 text-destructive" />
								{$t('common.delete')}
							</Button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
