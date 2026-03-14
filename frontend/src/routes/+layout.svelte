<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import { ModeWatcher, toggleMode } from 'mode-watcher';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Toaster } from '$lib/components/ui/sonner';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();

	const isLoginPage = $derived($page.url.pathname === '/login');
	const showSidebar = $derived(auth.isAuthenticated && !isLoginPage);

	let sidebarOpen = $state(false);

	onMount(async () => {
		await auth.check();
		if (auth.state === 'unauthenticated' && !isLoginPage) {
			goto('/login');
		}
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>
<ModeWatcher />
<Toaster />

{#if showSidebar}
	<div class="flex h-screen overflow-hidden bg-background">
		{#if sidebarOpen}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="fixed inset-0 z-20 bg-black/60 backdrop-blur-sm transition-opacity duration-200 md:hidden"
				onclick={() => (sidebarOpen = false)}
				onkeydown={(e) => e.key === 'Escape' && (sidebarOpen = false)}
			></div>
		{/if}

		<div
			class="fixed inset-y-0 left-0 z-30 transition-transform duration-250 ease-out md:static md:z-auto
			       {sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'}"
		>
			<Sidebar onClose={() => (sidebarOpen = false)} />
		</div>

		<main class="flex min-w-0 flex-1 flex-col overflow-hidden">
			<div class="flex h-12 shrink-0 items-center border-b border-border/50 px-4 md:hidden">
				<button
					class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground"
					onclick={() => (sidebarOpen = true)}
					aria-label="Open menu"
				>
					<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<line x1="3" y1="6" x2="21" y2="6" />
						<line x1="3" y1="12" x2="21" y2="12" />
						<line x1="3" y1="18" x2="21" y2="18" />
					</svg>
				</button>
				<span class="ml-3 text-sm font-bold tracking-widest uppercase text-foreground">Turboist</span>
				<Button onclick={toggleMode} variant="ghost" size="icon" class="ml-auto h-8 w-8 text-muted-foreground">
					<SunIcon class="h-4 w-4 scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
					<MoonIcon class="absolute h-4 w-4 scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
					<span class="sr-only">Toggle theme</span>
				</Button>
			</div>
			<div class="flex-1 overflow-y-auto">
				{@render children()}
			</div>
		</main>
	</div>
{:else}
	{@render children()}
{/if}
