<script lang="ts">
	import { auth } from '$lib/stores/auth.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import { t } from 'svelte-intl-precompile';

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
			error = $t('login.wrongPassword');
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-background">
	<div class="relative w-full max-w-xs px-6">
		<div class="absolute -top-32 left-1/2 h-64 w-64 -translate-x-1/2 rounded-full bg-primary/5 blur-3xl"></div>

		<div class="relative space-y-8">
			<div class="flex items-center justify-center gap-2.5">
				<ZapIcon class="h-5 w-5 text-primary" fill="currentColor" />
				<h1 class="text-lg font-bold tracking-widest uppercase text-foreground">Turboist</h1>
			</div>

			<form onsubmit={handleSubmit} class="space-y-5">
				<div class="space-y-2">
					<label for="password" class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
						{$t('login.password')}
					</label>
					<input
						id="password"
						type="password"
						bind:value={password}
						disabled={loading}
						autocomplete="current-password"
						placeholder="..."
						class="w-full rounded-lg border border-border bg-card px-3.5 py-2.5 text-sm text-foreground
							placeholder:text-muted-foreground/30
							focus:border-primary/50 focus:outline-none focus:ring-1 focus:ring-primary/30
							disabled:opacity-50 transition-all duration-200"
					/>
				</div>

				{#if error}
					<p class="text-[12px] text-destructive">{error}</p>
				{/if}

				<button
					type="submit"
					disabled={loading || !password}
					class="w-full rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-all duration-200
						hover:brightness-110 hover:shadow-lg hover:shadow-primary/20
						active:scale-[0.98]
						disabled:opacity-40 disabled:hover:shadow-none disabled:active:scale-100"
				>
					{loading ? $t('login.signingIn') : $t('login.signIn')}
				</button>
			</form>

			<p class="text-center text-[11px] text-muted-foreground/30">{$t('login.tagline')}</p>
		</div>
	</div>
</div>
