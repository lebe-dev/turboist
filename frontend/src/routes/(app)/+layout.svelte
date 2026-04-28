<script lang="ts">
	import Sidebar from '$lib/components/app/Sidebar.svelte';
	import Topbar from '$lib/components/app/Topbar.svelte';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';

	let { children } = $props();

	const auth = getAuthStore();

	let dataReady = $state(false);

	$effect(() => {
		if (auth.status === 'guest') void goto(resolve('/login'));
	});

	onMount(() => {
		if (auth.status !== 'authenticated') return;
		void (async () => {
			try {
				await Promise.all([
					configStore.load(),
					contextsStore.load(),
					projectsStore.load(),
					labelsStore.load()
				]);
				dataReady = true;
			} catch (err) {
				const message = err instanceof Error ? err.message : 'Failed to load workspace';
				toast.error(message);
			}
		})();
	});

	function onQuickAdd(): void {
		// QuickAddDialog is implemented in Task 4; placeholder until then.
		toast.info('Quick add coming soon');
	}

	function onKeydown(e: KeyboardEvent): void {
		const target = e.target as HTMLElement | null;
		if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
			return;
		}
		if (e.key === 'q' || e.key === 'Q') {
			e.preventDefault();
			onQuickAdd();
		} else if (e.key === '/') {
			e.preventDefault();
			void goto(resolve('/search'));
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if auth.status !== 'authenticated' || !dataReady}
	<div class="flex h-screen items-center justify-center text-sm text-muted-foreground">
		Loading workspace…
	</div>
{:else}
	<div class="flex h-screen overflow-hidden bg-background">
		<Sidebar />
		<div class="flex min-w-0 flex-1 flex-col">
			<Topbar {onQuickAdd} />
			<main class="flex-1 overflow-y-auto">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
