<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();

	const isLoginPage = $derived($page.url.pathname === '/login');
	const showSidebar = $derived(auth.isAuthenticated && !isLoginPage);

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
		<Sidebar />
		<main class="flex-1 overflow-y-auto">
			{@render children()}
		</main>
	</div>
{:else}
	{@render children()}
{/if}
