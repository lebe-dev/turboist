<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import { createAuthStore } from '$lib/auth/store.svelte';
	import { decideAuthRedirect } from '$lib/auth/guard';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { initI18n, t } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { getDB } from '$lib/offline/db';
	import { fetcherFromClient, flush, pull } from '$lib/offline/sync';
	import { createDexieAuthAdapter } from '$lib/offline/auth';

	initI18n(null);

	let { children } = $props();

	const authStore = createAuthStore({
		offline: createDexieAuthAdapter({
			onAuthenticatedRefresh: async () => {
				try {
					await flush(fetcherFromClient(getApiClient()), getDB());
				} catch {
					// Offline or transient — next online tick retries.
				}
			}
		})
	});

	let bootstrapped = $state(false);

	onMount(() => {
		void (async () => {
			await authStore.bootstrap();
			bootstrapped = true;
			if (authStore.status === 'authenticated') {
				void backgroundSyncPull();
			}
		})();
	});

	async function backgroundSyncPull(): Promise<void> {
		try {
			const db = getDB();
			const lastPulledAt = (await db.meta.get('lastPulledAt'))?.value as string | undefined;
			await pull(lastPulledAt, fetcherFromClient(getApiClient()), db);
		} catch {
			// Offline / unauthenticated / transient — silent: stores keep working from
			// REST and the next online tick will retry.
		}
	}

	$effect(() => {
		if (!bootstrapped) return;
		const redirect = decideAuthRedirect(authStore, page.url.pathname);
		if (redirect && redirect !== page.url.pathname) {
			void goto(resolve(redirect));
		}
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher />
<Toaster />

{#if !bootstrapped || authStore.status === 'loading'}
	<div class="flex h-screen items-center justify-center text-sm text-muted-foreground">
		{$t('app.loading')}
	</div>
{:else}
	{@render children()}
{/if}
