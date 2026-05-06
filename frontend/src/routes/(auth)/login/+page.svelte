<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { ApiError } from '$lib/api/errors';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { t } from '$lib/i18n';

	const auth = getAuthStore();

	let username = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

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
			await auth.login({ username, password });
			await goto(resolve('/'));
		} catch (err) {
			error =
				err instanceof ApiError ? err.message : err instanceof Error ? err.message : $t('auth.loginFailed');
		} finally {
			submitting = false;
		}
	}
</script>

<form class="flex flex-col gap-4" onsubmit={onSubmit}>
	<h1 class="text-lg font-semibold">{$t('auth.signIn')}</h1>
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
</form>
