<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { ApiError } from '$lib/api/errors';
	import { loadPublicConfig } from '$lib/api';
	import { isPasskeySupported, isPasskeyCancellation } from '$lib/webauthn';
	import { onMount } from 'svelte';
	import KeyIcon from 'phosphor-svelte/lib/Key';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { t } from '$lib/i18n';

	const auth = getAuthStore();

	let username = $state('');
	let password = $state('');
	let otpCode = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let useRecovery = $state(false);
	// The passkey button appears only when this runtime can run a ceremony AND
	// the instance actually has a credential to match — otherwise it would open
	// an empty platform picker.
	let passkeyOffered = $state(false);
	let passkeySubmitting = $state(false);

	onMount(async () => {
		if (!isPasskeySupported()) return;
		const config = await loadPublicConfig();
		passkeyOffered = !!config?.passkeys?.available;
	});

	$effect(() => {
		if (auth.setupRequired) void goto(resolve('/setup'));
		else if (auth.status === 'authenticated') void goto(resolve('/'));
	});

	async function onSubmit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (submitting) return;
		submitting = true;
		error = null;
		try {
			const res = await auth.login({ username, password });
			if (res.otpRequired) {
				otpCode = '';
				return;
			}
			await goto(resolve('/'));
		} catch (err) {
			error =
				err instanceof ApiError ? err.message : err instanceof Error ? err.message : $t('auth.loginFailed');
		} finally {
			submitting = false;
		}
	}

	async function onOtpSubmit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (submitting) return;
		submitting = true;
		error = null;
		try {
			await auth.verifyOtp(otpCode.trim());
			await goto(resolve('/'));
		} catch (err) {
			if (
				err instanceof ApiError &&
				(err.code === 'totp_invalid_code' || err.code === 'auth_invalid')
			) {
				error = $t('auth.otpFailed');
			} else {
				error = err instanceof Error ? err.message : $t('auth.otpFailed');
			}
		} finally {
			submitting = false;
		}
	}

	async function onPasskeySignIn(): Promise<void> {
		if (passkeySubmitting) return;
		passkeySubmitting = true;
		error = null;
		try {
			await auth.loginWithPasskey();
			await goto(resolve('/'));
		} catch (err) {
			// Closing the platform sheet is not a failure worth shouting about.
			if (isPasskeyCancellation(err)) return;
			error =
				err instanceof ApiError
					? err.message
					: err instanceof Error
						? err.message
						: $t('auth.passkeyFailed');
		} finally {
			passkeySubmitting = false;
		}
	}

	function onCancelOtp(): void {
		auth.cancelOtp();
		otpCode = '';
		error = null;
		useRecovery = false;
	}

	function switchToRecovery(): void {
		useRecovery = true;
		otpCode = '';
		error = null;
	}

	function switchToApp(): void {
		useRecovery = false;
		otpCode = '';
		error = null;
	}
</script>

{#if auth.awaitingOtp}
	<form class="flex flex-col gap-4" onsubmit={onOtpSubmit}>
		<h1 class="text-lg font-semibold">{$t('auth.otpTitle')}</h1>
		<p class="text-sm text-muted-foreground">
			{useRecovery ? $t('auth.otpRecoveryHint') : $t('auth.otpAppHint')}
		</p>
		<div class="flex flex-col gap-1.5">
			<Label for="otp">{useRecovery ? $t('auth.otpRecoveryLabel') : $t('auth.otpAppLabel')}</Label>
			<Input
				id="otp"
				bind:value={otpCode}
				autocomplete={useRecovery ? 'off' : 'one-time-code'}
				inputmode={useRecovery ? 'text' : 'numeric'}
				placeholder={useRecovery ? 'XXXXXXXXXX' : '000000'}
				autofocus
				required
			/>
		</div>
		{#if error}
			<p class="text-xs text-destructive">{error}</p>
		{/if}
		<Button type="submit" disabled={submitting}>
			{submitting ? $t('auth.signingIn') : $t('auth.otpVerify')}
		</Button>
		{#if useRecovery}
			<button type="button" class="text-xs text-muted-foreground underline" onclick={switchToApp}>
				{$t('auth.otpUseApp')}
			</button>
		{:else}
			<button
				type="button"
				class="text-xs text-muted-foreground underline"
				onclick={switchToRecovery}
			>
				{$t('auth.otpUseRecovery')}
			</button>
		{/if}
		<button type="button" class="text-xs text-muted-foreground underline" onclick={onCancelOtp}>
			{$t('auth.otpCancel')}
		</button>
	</form>
{:else}
	<form class="flex flex-col gap-4" onsubmit={onSubmit}>
		<div class="flex flex-col gap-1.5">
			<Label for="username">{$t('auth.username')}</Label>
			<Input id="username" bind:value={username} autocomplete="username" required />
		</div>
		<div class="flex flex-col gap-1.5">
			<Label for="password">{$t('auth.password')}</Label>
			<Input
				id="password"
				type="password"
				bind:value={password}
				autocomplete="current-password"
				required
			/>
		</div>
		{#if error}
			<p class="text-xs text-destructive">{error}</p>
		{/if}
		<Button type="submit" disabled={submitting}>
			{submitting ? $t('auth.signingIn') : $t('auth.signIn')}
		</Button>
		{#if passkeyOffered}
			<div class="flex items-center gap-3 text-xs text-muted-foreground">
				<span class="h-px flex-1 bg-border"></span>
				{$t('auth.passkeyOr')}
				<span class="h-px flex-1 bg-border"></span>
			</div>
			<Button
				type="button"
				variant="outline"
				onclick={onPasskeySignIn}
				disabled={passkeySubmitting || submitting}
			>
				<KeyIcon class="size-4" />
				{passkeySubmitting ? $t('auth.passkeySigningIn') : $t('auth.passkeySignIn')}
			</Button>
		{/if}
	</form>
{/if}

<div class="mt-6 flex justify-center gap-3 text-xs text-muted-foreground">
	<a href={resolve('/terms-of-service')} class="hover:text-foreground hover:underline">
		{$t('legal.tos.title')}
	</a>
	<span aria-hidden="true">·</span>
	<a href={resolve('/privacy-policy')} class="hover:text-foreground hover:underline">
		{$t('legal.privacy.title')}
	</a>
</div>
