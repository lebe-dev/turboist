<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { ApiError } from '$lib/api/errors';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

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
			error = 'Passwords do not match';
			return;
		}
		submitting = true;
		error = null;
		try {
			await auth.setup({ username, password });
			await goto(resolve('/'));
		} catch (err) {
			error =
				err instanceof ApiError ? err.message : err instanceof Error ? err.message : 'Setup failed';
		} finally {
			submitting = false;
		}
	}
</script>

<form class="flex flex-col gap-4" onsubmit={onSubmit}>
	<h1 class="text-lg font-semibold">Create the first account</h1>
	<p class="text-xs text-muted-foreground">
		Turboist runs single-user. Pick a username and password.
	</p>
	<div class="flex flex-col gap-1.5">
		<Label for="username">Username</Label>
		<Input id="username" bind:value={username} autocomplete="username" required />
	</div>
	<div class="flex flex-col gap-1.5">
		<Label for="password">Password</Label>
		<Input
			id="password"
			type="password"
			bind:value={password}
			autocomplete="new-password"
			required
		/>
	</div>
	<div class="flex flex-col gap-1.5">
		<Label for="confirm">Confirm password</Label>
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
		{submitting ? 'Creating…' : 'Create account'}
	</Button>
</form>
