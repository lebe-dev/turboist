<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
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

{#if showSidebar}
	<div class="flex h-screen overflow-hidden bg-background">
		{#if sidebarOpen}
			<div
				class="fixed inset-0 z-20 bg-black/40 md:hidden"
				onclick={() => (sidebarOpen = false)}
			></div>
		{/if}

		<div
			class="fixed inset-y-0 left-0 z-30 transition-transform duration-200 md:static md:z-auto
			       {sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'}"
		>
			<Sidebar onClose={() => (sidebarOpen = false)} />
		</div>

		<main class="flex min-w-0 flex-1 flex-col overflow-hidden">
			<div class="flex h-14 shrink-0 items-center border-b border-border px-4 md:hidden">
				<button
					class="flex h-9 w-9 items-center justify-center rounded-md hover:bg-accent"
					onclick={() => (sidebarOpen = true)}
					aria-label="Открыть меню"
				>
					<svg
						xmlns="http://www.w3.org/2000/svg"
						width="20"
						height="20"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<line x1="3" y1="6" x2="21" y2="6" />
						<line x1="3" y1="12" x2="21" y2="12" />
						<line x1="3" y1="18" x2="21" y2="18" />
					</svg>
				</button>
				<span class="ml-3 text-base font-semibold">Turboist</span>
			</div>
			<div class="flex-1 overflow-y-auto">
				{@render children()}
			</div>
		</main>
	</div>
{:else}
	{@render children()}
{/if}
