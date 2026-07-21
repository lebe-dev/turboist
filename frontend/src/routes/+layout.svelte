<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import { toast } from 'svelte-sonner';
	import { createAuthStore, type AuthStore } from '$lib/auth/store.svelte';
	import { decideAuthRedirect } from '$lib/auth/guard';
	import { beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page, updated } from '$app/state';
	import { onMount } from 'svelte';
	import { initI18n, t } from '$lib/i18n';
	import { isNativePlatform } from '$lib/native/platform';
	import { initDeepLinks } from '$lib/native/deepLink';
	import { loadServerUrl, saveServerUrl } from '$lib/native/serverUrl';
	import { shouldForceReload } from '$lib/sw/reload';
	import { statusStore } from '$lib/offline';
	import { initSentry } from '$lib/observability/sentry';
	import ConnectServer from '$lib/components/ConnectServer.svelte';

	initI18n(null);

	let { children } = $props();

	const native = isNativePlatform();
	let authStore = $state<AuthStore | null>(null);
	let bootstrapped = $state(false);
	let needsServer = $state(false);

	// When a new deploy has been detected (version polling in svelte.config.js),
	// force a full-page navigation instead of a client-side one — the old tab's
	// chunk hashes no longer exist, so a SPA navigation would crash the router
	// with "Cannot read properties of undefined (reading 'universal')".
	// Web-only: the native bundle is local and version.json never flips.
	// §5.3: gate the forced reload on `statusStore.online` — reloading while
	// offline (before the service worker is active) would white-screen, and with
	// the SW active it is pointless. The 60s version poll itself is untouched.
	beforeNavigate((nav) => {
		const target = nav.to?.url;
		const force = shouldForceReload({
			native,
			updated: updated.current,
			willUnload: nav.willUnload,
			hasTarget: Boolean(target),
			online: statusStore.online
		});
		if (force && target) {
			nav.cancel();
			window.location.href = target.href;
		}
	});

	// Show a toast as soon as a new deploy is detected so the user can reload
	// before navigating — this closes the race window where the version.json
	// fetch resolves and the user clicks a link in the same tick, causing
	// beforeNavigate to see updated.current = false and then crash mid-flight.
	$effect(() => {
		if (native || !updated.current) return;
		toast.info($t('app.newVersionAvailable'), {
			duration: Infinity,
			action: {
				label: $t('app.reload'),
				onClick: () => window.location.reload()
			}
		});
	});

	async function boot() {
		const serverUrl = await loadServerUrl(); // '' on web
		if (native && !serverUrl) {
			needsServer = true;
			return;
		}
		// Native: the server URL is now known, so telemetry config can be fetched.
		// Web: already initialised from hooks.client.ts (guarded no-op).
		await initSentry();
		authStore = createAuthStore({ baseUrl: serverUrl || undefined });
		await authStore.bootstrap();
		bootstrapped = true;
	}

	async function onConnect(url: string) {
		await saveServerUrl(url);
		needsServer = false;
		await boot();
	}

	onMount(() => {
		void boot();

		// Register the native deep-link listener (lock-screen widget → QuickAdd).
		// No-op on web.
		void initDeepLinks();

		// The chunk-reload recovery below is web-only: native assets are bundled
		// and never 404 on a stale hash.
		if (native) return;

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
		if (!bootstrapped || !authStore) return;
		const redirect = decideAuthRedirect(authStore, page.url.pathname);
		if (redirect && redirect !== page.url.pathname) {
			void goto(resolve(redirect));
		}
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher />
<Toaster />

{#if needsServer}
	<ConnectServer onconnect={onConnect} />
{:else if !bootstrapped || !authStore || authStore.status === 'loading'}
	<div class="flex h-screen items-center justify-center text-sm text-muted-foreground">
		{$t('app.loading')}
	</div>
{:else}
	{@render children()}
{/if}
