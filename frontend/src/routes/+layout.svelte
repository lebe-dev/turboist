<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth.svelte';
	import { contextsStore, type View } from '$lib/stores/contexts.svelte';
	import { planningStore } from '$lib/stores/planning.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import { t } from 'svelte-intl-precompile';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import { initLocale } from '$lib/i18n';
	import UpdateBanner from '$lib/components/UpdateBanner.svelte';
	import InstallBanner from '$lib/components/InstallBanner.svelte';
	import AutoRemovePausedBanner from '$lib/components/AutoRemovePausedBanner.svelte';
	import QuickCaptureButton from '$lib/components/QuickCaptureButton.svelte';
	import ConnectionIndicator from '$lib/components/ConnectionIndicator.svelte';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	initLocale();

	let { children } = $props();

	const isLoginPage = $derived($page.url.pathname === '/login');
	const showSidebar = $derived(auth.isAuthenticated && !isLoginPage);

	let sidebarOpen = $state(false);

	onMount(() => {
		auth.check().then(() => {
			if (auth.state === 'unauthenticated' && !isLoginPage) {
				goto('/login');
			}
		});
	});

	// Reactive: init app whenever auth becomes authenticated (handles login navigation)
	$effect(() => {
		if (auth.isAuthenticated && !appStore.initialized) {
			appStore.init();
		}
	});

	// Tear down app resources on logout
	$effect(() => {
		if (auth.state === 'unauthenticated' && appStore.initialized) {
			appStore.destroy();
		}
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>
<ModeWatcher />
<Toaster />
<UpdateBanner />
<InstallBanner />
<AutoRemovePausedBanner />

<QuickCaptureButton showButton={false} />

{#if showSidebar}
    <div class="flex h-screen overflow-hidden bg-background">
        {#if sidebarOpen}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
                class="fixed inset-0 z-20 bg-black/60 backdrop-blur-sm transition-opacity duration-200 md:hidden"
                onclick={() => (sidebarOpen = false)}
                onkeydown={(e) => e.key === "Escape" && (sidebarOpen = false)}
            ></div>
        {/if}

        <div
            class="fixed inset-y-0 left-0 z-30 transition-transform duration-250 ease-out md:static md:z-auto
			       {sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'}"
        >
            <Sidebar onClose={() => (sidebarOpen = false)} />
        </div>

        <main class="flex min-w-0 flex-1 flex-col overflow-hidden">
            <div
                class="flex h-12 shrink-0 items-center border-b border-border/50 px-4 md:hidden"
            >
                <button
                    class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground"
                    onclick={() => (sidebarOpen = true)}
                    aria-label="Open menu"
                >
                    <svg
                        xmlns="http://www.w3.org/2000/svg"
                        width="18"
                        height="18"
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
                <div class="ml-2 flex gap-1">
                    {#each [{ id: "today", key: "views.today" }, { id: "tomorrow", key: "views.tomorrow" }, { id: "weekly", key: "views.weekly" }] as view (view.id)}
                        <button
                            class="rounded-md px-2.5 py-1 text-xs font-semibold transition-colors duration-150
							{contextsStore.activeView === view.id
                                ? 'bg-accent text-foreground'
                                : 'text-muted-foreground hover:text-foreground'}"
                            onclick={() => {
                                if (planningStore.active) planningStore.exit();
                                if ($page.url.pathname !== "/") goto("/");
                                contextsStore.setView(view.id as View);
                            }}
                        >
                            {$t(view.key)}
                        </button>
                    {/each}
                </div>
                <div class="ml-auto">
                    <ConnectionIndicator compact />
                </div>
            </div>
            <div class="flex-1 overflow-y-auto">
                {@render children()}
            </div>
        </main>
    </div>
{:else}
    {@render children()}
{/if}
