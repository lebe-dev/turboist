<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import DesktopIcon from 'phosphor-svelte/lib/Desktop';
	import DeviceMobileIcon from 'phosphor-svelte/lib/DeviceMobile';
	import TerminalIcon from 'phosphor-svelte/lib/Terminal';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { t, locale } from '$lib/i18n';
	import { getApiClient, sessions, auth, type Session } from '$lib/api';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	const authStore = getAuthStore();

	let items = $state<Session[]>([]);
	let loading = $state(true);
	let revokingId = $state<number | null>(null);
	let logoutOthersBusy = $state(false);
	let logoutAllBusy = $state(false);
	let pendingRevokeId = $state<number | null>(null);
	let revokeOpen = $state(false);
	let logoutOthersOpen = $state(false);
	let logoutAllOpen = $state(false);

	const hasOthers = $derived(items.some((s) => !s.isCurrent));

	onMount(load);

	async function load(): Promise<void> {
		const client = getApiClient();
		if (!client) return;
		loading = true;
		try {
			items = await sessions.list(client);
		} catch (err) {
			toast.error(describeError(err, $t('settings.sessions.loadFailed')));
		} finally {
			loading = false;
		}
	}

	function askRevoke(id: number): void {
		pendingRevokeId = id;
		revokeOpen = true;
	}

	async function onConfirmRevoke(): Promise<void> {
		if (pendingRevokeId == null) return;
		const client = getApiClient();
		if (!client) return;
		const id = pendingRevokeId;
		revokingId = id;
		try {
			await sessions.revoke(client, id);
			items = items.filter((s) => s.id !== id);
		} catch (err) {
			toast.error(describeError(err, $t('settings.sessions.revokeFailed')));
		} finally {
			revokingId = null;
			pendingRevokeId = null;
			revokeOpen = false;
		}
	}

	async function onConfirmLogoutOthers(): Promise<void> {
		if (logoutOthersBusy) return;
		const client = getApiClient();
		if (!client) return;
		logoutOthersBusy = true;
		try {
			await auth.logoutOthers(client);
			items = items.filter((s) => s.isCurrent);
			toast.success($t('settings.sessions.logoutOthersDone'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.sessions.logoutOthersFailed')));
		} finally {
			logoutOthersBusy = false;
			logoutOthersOpen = false;
		}
	}

	async function onConfirmLogoutAll(): Promise<void> {
		if (logoutAllBusy) return;
		logoutAllBusy = true;
		try {
			await authStore.logoutAll();
			await goto(resolve('/login'));
		} catch {
			toast.error($t('settings.session.logoutAllFailed'));
		} finally {
			logoutAllBusy = false;
			logoutAllOpen = false;
		}
	}

	function clientIcon(kind: Session['clientKind']): typeof DesktopIcon {
		switch (kind) {
			case 'ios':
				return DeviceMobileIcon;
			case 'cli':
				return TerminalIcon;
			default:
				return DesktopIcon;
		}
	}

	function clientKindLabel(kind: Session['clientKind']): string {
		switch (kind) {
			case 'ios':
				return $t('settings.sessions.clientIos');
			case 'cli':
				return $t('settings.sessions.clientCli');
			default:
				return $t('settings.sessions.clientWeb');
		}
	}

	// Renders "5 minutes ago" / "in 3 days" using Intl.RelativeTimeFormat.
	// Falls back to the raw ISO string when the value cannot be parsed.
	function formatRelative(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		const diffSec = Math.round((d.getTime() - Date.now()) / 1000);
		const absSec = Math.abs(diffSec);
		const rtf = new Intl.RelativeTimeFormat($locale || 'en', { numeric: 'auto' });
		const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
			['year', 60 * 60 * 24 * 365],
			['month', 60 * 60 * 24 * 30],
			['week', 60 * 60 * 24 * 7],
			['day', 60 * 60 * 24],
			['hour', 60 * 60],
			['minute', 60]
		];
		for (const [unit, sec] of units) {
			if (absSec >= sec) {
				return rtf.format(Math.round(diffSec / sec), unit);
			}
		}
		return rtf.format(diffSec, 'second');
	}

	function formatAbsolute(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.sessions.heading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.sessions.description')}</p>
	</div>

	{#if loading}
		<div class="text-xs text-muted-foreground">…</div>
	{:else if items.length === 0}
		<p class="text-xs text-muted-foreground">{$t('settings.sessions.empty')}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each items as s (s.id)}
				{@const Icon = clientIcon(s.clientKind)}
				<li
					class="flex items-start justify-between gap-3 rounded-md border border-border bg-background px-3 py-2.5"
				>
					<div class="flex min-w-0 flex-1 items-start gap-3">
						<Icon class="size-4 shrink-0 text-muted-foreground" />
						<div class="flex min-w-0 flex-1 flex-col gap-0.5">
							<div class="flex flex-wrap items-center gap-2">
								<span class="truncate text-sm font-medium">
									{clientKindLabel(s.clientKind)} · {s.displayName}
								</span>
								{#if s.isCurrent}
									<span
										class="inline-flex items-center rounded-full bg-foreground/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-foreground/80"
									>
										{$t('settings.sessions.current')}
									</span>
								{/if}
							</div>
							<span class="text-xs text-muted-foreground" title={formatAbsolute(s.lastUsedAt)}>
								{$t('settings.sessions.lastActive')}: {formatRelative(s.lastUsedAt)}
							</span>
							<span class="text-[11px] text-muted-foreground/80" title={formatAbsolute(s.createdAt)}>
								{$t('settings.sessions.signedIn')}: {formatRelative(s.createdAt)}
							</span>
							{#if s.ipAddress}
								<span class="font-mono text-[11px] text-muted-foreground/80">
									{$t('settings.sessions.ipAddress')}: {s.ipAddress}
								</span>
							{/if}
							{#if s.userAgent}
								<details class="group mt-1 text-[11px] text-muted-foreground/80">
									<summary
										class="cursor-pointer list-none select-none text-muted-foreground transition-colors hover:text-foreground"
									>
										<span class="inline group-open:hidden"
											>{$t('settings.sessions.showUserAgent')}</span
										>
										<span class="hidden group-open:inline"
											>{$t('settings.sessions.hideUserAgent')}</span
										>
									</summary>
									<code
										class="mt-1 block break-all rounded-md border border-border bg-muted/30 px-2 py-1 font-mono text-[11px] leading-snug"
										>{s.userAgent}</code
									>
								</details>
							{/if}
						</div>
					</div>
					{#if !s.isCurrent}
						<button
							type="button"
							class="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
							onclick={() => askRevoke(s.id)}
							disabled={revokingId === s.id}
						>
							{revokingId === s.id
								? $t('settings.sessions.revoking')
								: $t('settings.sessions.revoke')}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex flex-wrap gap-2 pt-1">
		{#if hasOthers}
			<Button
				type="button"
				variant="outline"
				size="sm"
				onclick={() => (logoutOthersOpen = true)}
				disabled={logoutOthersBusy}
			>
				<SignOutIcon class="size-4" />
				{logoutOthersBusy
					? $t('settings.sessions.loggingOutOthers')
					: $t('settings.sessions.logoutOthers')}
			</Button>
		{/if}
		<Button
			type="button"
			variant="outline"
			size="sm"
			onclick={() => (logoutAllOpen = true)}
			disabled={logoutAllBusy}
		>
			<SignOutIcon class="size-4" />
			{logoutAllBusy ? $t('settings.session.loggingOut') : $t('settings.session.logoutAll')}
		</Button>
	</div>
</section>

<AlertDialog.Root bind:open={revokeOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.sessions.confirmRevokeTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.sessions.confirmRevokeDescription')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onConfirmRevoke}>
				{$t('settings.sessions.revoke')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={logoutOthersOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.sessions.confirmLogoutOthersTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.sessions.confirmLogoutOthersDescription')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onConfirmLogoutOthers}>
				{$t('settings.sessions.logoutOthers')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={logoutAllOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.sessions.confirmLogoutAllTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.sessions.confirmLogoutAllDescription')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onConfirmLogoutAll}>
				{$t('settings.session.logoutAll')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
