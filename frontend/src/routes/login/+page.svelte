<script lang="ts">
	import { auth } from '$lib/stores/auth.svelte';

	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			await auth.login(password);
		} catch {
			error = 'Неверный пароль';
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center">
	<div class="w-full max-w-sm space-y-6 p-8">
		<h1 class="text-2xl font-semibold">Turboist</h1>

		<form onsubmit={handleSubmit} class="space-y-4">
			<div class="space-y-2">
				<label for="password" class="text-sm font-medium">Пароль</label>
				<input
					id="password"
					type="password"
					bind:value={password}
					disabled={loading}
					autocomplete="current-password"
					class="w-full rounded-md border px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
				/>
			</div>

			{#if error}
				<p class="text-sm text-destructive">{error}</p>
			{/if}

			<button
				type="submit"
				disabled={loading || !password}
				class="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
			>
				{loading ? 'Вход...' : 'Войти'}
			</button>
		</form>
	</div>
</div>
