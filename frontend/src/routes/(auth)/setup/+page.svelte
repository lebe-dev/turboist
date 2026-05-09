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
	let confirm = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	$effect(() => {
		if (!auth.setupRequired && auth.status === 'guest') void goto(resolve('/login'));
		else if (auth.status === 'authenticated') void goto(resolve('/'));
	});

	async function onSubmit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (submitting) return;
		if (password !== confirm) {
			error = $t('auth.passwordsMismatch');
			return;
		}
		submitting = true;
		error = null;
		try {
			await auth.setup({ username, password });
			await goto(resolve('/'));
		} catch (err) {
			error =
				err instanceof ApiError ? err.message : err instanceof Error ? err.message : $t('auth.setupFailed');
		} finally {
			submitting = false;
		}
	}
</script>

<form class="flex flex-col gap-4" onsubmit={onSubmit}>
	<h1 class="text-lg font-semibold">{$t('auth.setupTitle')}</h1>
	<p class="text-xs text-muted-foreground">{$t('auth.setupSubtitle')}</p>
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
			autocomplete="new-password"
			required
		/>
	</div>
	<div class="flex flex-col gap-1.5">
		<Label for="confirm">{$t('auth.confirmPassword')}</Label>
		<Input
			id="confirm"
			type="password"
			bind:value={confirm}
			autocomplete="new-password"
			required
		/>
	</div>
	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}
	<Button type="submit" disabled={submitting}>
		{submitting ? $t('auth.creating') : $t('auth.createAccount')}
	</Button>
</form>
