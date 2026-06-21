<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import { createAuthStore } from '$lib/auth/store.svelte';
	import { decideAuthRedirect } from '$lib/auth/guard';
	import { beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page, updated } from '$app/state';
	import { onMount } from 'svelte';
	import { initI18n, t } from '$lib/i18n';

	initI18n(null);

	let { children } = $props();

	const authStore = createAuthStore();

	// When a new deploy has been detected (version polling in svelte.config.js),
	// force a full-page navigation instead of a client-side one — the old tab's
	// chunk hashes no longer exist, so a SPA navigation would crash the router
	// with "Cannot read properties of undefined (reading 'universal')".
	beforeNavigate((nav) => {
		if (updated.current && !nav.willUnload && nav.to?.url) {
			nav.cancel();
			window.location.href = nav.to.url.href;
		}
	});

	let bootstrapped = $state(false);

	onMount(() => {
		void (async () => {
			await authStore.bootstrap();
			bootstrapped = true;
		})();

		const RELOAD_GUARD_KEY = 'turboist:chunk-reload';
		// Clear the guard once the app has run a bit, so a *future* deploy can
		// trigger a reload again — this only prevents a tight reload loop.
		setTimeout(() => sessionStorage.removeItem(RELOAD_GUARD_KEY), 5000);

		const onPreloadError = (event: Event) => {
			event.preventDefault();
			if (sessionStorage.getItem(RELOAD_GUARD_KEY)) return;
			sessionStorage.setItem(RELOAD_GUARD_KEY, '1');
			window.location.reload();
		};
		window.addEventListener('vite:preloadError', onPreloadError);
		return () => window.removeEventListener('vite:preloadError', onPreloadError);
	});

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
