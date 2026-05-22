<script lang="ts">
	import { toast } from 'svelte-sonner';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import DownloadIcon from 'phosphor-svelte/lib/DownloadSimple';
	import ShieldCheckIcon from 'phosphor-svelte/lib/ShieldCheck';
	import { t } from '$lib/i18n';
	import { getApiClient, totp } from '$lib/api';
	import { ApiError } from '$lib/api/errors';
	import { describeError } from '$lib/utils/taskActions';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';

	type Mode = 'idle' | 'setup' | 'recovery' | 'disable';

	let { available = true }: { available?: boolean } = $props();

	const auth = getAuthStore();
	const enabled = $derived(auth.user?.totpEnabled ?? false);

	let mode = $state<Mode>('idle');
	let busy = $state(false);
	let secret = $state('');
	let qrPngBase64 = $state('');
	let code = $state('');
	let recoveryCodes = $state<string[]>([]);

	async function startSetup(): Promise<void> {
		const client = getApiClient();
		if (!client || busy) return;
		busy = true;
		try {
			const res = await totp.setup(client);
			secret = res.secret;
			qrPngBase64 = res.qrPngBase64;
			code = '';
			mode = 'setup';
		} catch (err) {
			toast.error(describeError(err, $t('settings.twofa.setupFailed')));
		} finally {
			busy = false;
		}
	}

	async function confirmSetup(): Promise<void> {
		const trimmed = code.trim();
		if (!trimmed || busy) return;
		const client = getApiClient();
		if (!client) return;
		busy = true;
		try {
			const res = await totp.confirm(client, trimmed);
			recoveryCodes = res.recoveryCodes;
			if (auth.user) {
				auth.user = { ...auth.user, totpEnabled: true };
			}
			code = '';
			mode = 'recovery';
		} catch (err) {
			toast.error(describeError(err, $t('settings.twofa.confirmFailed')));
		} finally {
			busy = false;
		}
	}

	function cancelSetup(): void {
		if (busy) return;
		mode = 'idle';
		secret = '';
		qrPngBase64 = '';
		code = '';
	}

	function startDisable(): void {
		code = '';
		mode = 'disable';
	}

	function cancelDisable(): void {
		if (busy) return;
		mode = 'idle';
		code = '';
	}

	async function confirmDisable(): Promise<void> {
		const trimmed = code.trim();
		if (!trimmed || busy) return;
		const client = getApiClient();
		if (!client) return;
		busy = true;
		try {
			await totp.disable(client, trimmed);
			if (auth.user) {
				auth.user = { ...auth.user, totpEnabled: false };
			}
			toast.success($t('settings.twofa.disabledToast'));
			mode = 'idle';
			code = '';
		} catch (err) {
			if (err instanceof ApiError && err.code === 'totp_invalid_code') {
				toast.error($t('settings.twofa.invalidCode'));
			} else {
				toast.error(describeError(err, $t('settings.twofa.disableFailed')));
			}
		} finally {
			busy = false;
		}
	}

	function finishRecovery(): void {
		recoveryCodes = [];
		secret = '';
		qrPngBase64 = '';
		mode = 'idle';
		toast.success($t('settings.twofa.enabledToast'));
	}

	async function copySecret(): Promise<void> {
		if (!secret) return;
		try {
			await navigator.clipboard.writeText(secret);
			toast.success($t('settings.twofa.secretCopied'));
		} catch {
			// clipboard may be blocked
		}
	}

	async function copyRecoveryCodes(): Promise<void> {
		if (recoveryCodes.length === 0) return;
		try {
			await navigator.clipboard.writeText(recoveryCodes.join('\n'));
			toast.success($t('settings.twofa.recoveryCopied'));
		} catch {
			// clipboard may be blocked
		}
	}

	function downloadRecoveryCodes(): void {
		if (recoveryCodes.length === 0) return;
		const blob = new Blob([recoveryCodes.join('\n') + '\n'], { type: 'text/plain' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'turboist-recovery-codes.txt';
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
		URL.revokeObjectURL(url);
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.twofa.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.twofa.description')}</p>
		</div>
		{#if enabled}
			<span
				class="inline-flex shrink-0 items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
				data-testid="twofa-status"
			>
				<ShieldCheckIcon class="size-3.5" weight="fill" />
				{$t('settings.twofa.statusEnabled')}
			</span>
		{/if}
	</div>

	{#if !available}
		<p
			data-testid="twofa-unavailable"
			class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200"
		>
			{$t('settings.twofa.unavailable')}
		</p>
	{/if}

	{#if mode === 'idle'}
		{#if enabled}
			<div>
				<Button type="button" variant="outline" onclick={startDisable} disabled={busy}>
					{$t('settings.twofa.disableButton')}
				</Button>
			</div>
		{:else}
			<div>
				<Button
					type="button"
					variant="secondary"
					onclick={startSetup}
					disabled={busy || !available}
				>
					{busy ? $t('settings.twofa.starting') : $t('settings.twofa.enableButton')}
				</Button>
			</div>
		{/if}
	{:else if mode === 'setup'}
		<div class="flex flex-col gap-3">
			<p class="text-xs text-muted-foreground">{$t('settings.twofa.setupHint')}</p>
			{#if qrPngBase64}
				<div class="flex justify-center">
					<img
						src={`data:image/png;base64,${qrPngBase64}`}
						alt={$t('settings.twofa.qrAlt')}
						class="size-44 rounded-md border border-border bg-white p-2"
					/>
				</div>
			{/if}
			<div class="flex flex-col gap-1">
				<span class="text-[11px] font-medium text-muted-foreground"
					>{$t('settings.twofa.secretLabel')}</span
				>
				<div class="flex items-center gap-2">
					<code
						data-testid="twofa-secret"
						class="flex-1 break-all rounded-md border border-border bg-muted/40 px-2 py-1.5 font-mono text-xs"
						>{secret}</code
					>
					<Button type="button" variant="outline" size="sm" onclick={copySecret}>
						<CopyIcon class="size-4" />
					</Button>
				</div>
			</div>
			<form
				class="flex flex-col gap-2 sm:flex-row sm:items-center"
				onsubmit={(e) => {
					e.preventDefault();
					confirmSetup();
				}}
			>
				<Input
					type="text"
					inputmode="numeric"
					autocomplete="one-time-code"
					placeholder={$t('settings.twofa.codePlaceholder')}
					bind:value={code}
					disabled={busy}
					maxlength={10}
					class="sm:max-w-xs"
				/>
				<div class="flex gap-2">
					<Button type="submit" variant="default" disabled={busy || code.trim() === ''}>
						{busy ? $t('settings.twofa.confirming') : $t('settings.twofa.confirmButton')}
					</Button>
					<Button type="button" variant="ghost" onclick={cancelSetup} disabled={busy}>
						{$t('common.cancel')}
					</Button>
				</div>
			</form>
		</div>
	{:else if mode === 'recovery'}
		<div class="flex flex-col gap-3">
			<p class="text-xs text-muted-foreground">{$t('settings.twofa.recoveryHint')}</p>
			<ul
				data-testid="twofa-recovery-codes"
				class="grid grid-cols-2 gap-1.5 rounded-md border border-border bg-muted/40 p-3 font-mono text-xs"
			>
				{#each recoveryCodes as rc (rc)}
					<li class="select-all">{rc}</li>
				{/each}
			</ul>
			<div class="flex flex-wrap gap-2">
				<Button type="button" variant="outline" size="sm" onclick={copyRecoveryCodes}>
					<CopyIcon class="size-4" />
					{$t('settings.twofa.copyRecovery')}
				</Button>
				<Button type="button" variant="outline" size="sm" onclick={downloadRecoveryCodes}>
					<DownloadIcon class="size-4" />
					{$t('settings.twofa.downloadRecovery')}
				</Button>
				<Button type="button" variant="default" size="sm" onclick={finishRecovery}>
					{$t('settings.twofa.finishRecovery')}
				</Button>
			</div>
		</div>
	{:else if mode === 'disable'}
		<form
			class="flex flex-col gap-2 sm:flex-row sm:items-center"
			onsubmit={(e) => {
				e.preventDefault();
				confirmDisable();
			}}
		>
			<Input
				type="text"
				inputmode="numeric"
				autocomplete="one-time-code"
				placeholder={$t('settings.twofa.codeOrRecoveryPlaceholder')}
				bind:value={code}
				disabled={busy}
				maxlength={32}
				class="sm:max-w-xs"
			/>
			<div class="flex gap-2">
				<Button type="submit" variant="destructive" disabled={busy || code.trim() === ''}>
					{busy ? $t('settings.twofa.disabling') : $t('settings.twofa.disableConfirm')}
				</Button>
				<Button type="button" variant="ghost" onclick={cancelDisable} disabled={busy}>
					{$t('common.cancel')}
				</Button>
			</div>
		</form>
	{/if}
</section>
