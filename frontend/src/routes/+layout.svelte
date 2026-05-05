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

	let { children } = $props();

	const authStore = createAuthStore();

	let bootstrapped = $state(false);

	onMount(() => {
		void (async () => {
			await authStore.bootstrap();
			bootstrapped = true;
		})();
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
		Loading…
	</div>
{:else}
	{@render children()}
{/if}
