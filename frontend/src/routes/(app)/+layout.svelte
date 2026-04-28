<script lang="ts">
	import Sidebar from '$lib/components/app/Sidebar.svelte';
	import Topbar from '$lib/components/app/Topbar.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { decideAuthRedirect } from '$lib/auth/guard';
	import { page } from '$app/state';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { describeError } from '$lib/utils/taskActions';
	import type { TaskInput } from '$lib/api/types';

	let { children } = $props();

	const auth = getAuthStore();

	let dataReady = $state(false);
	let loadStarted = $state(false);
	let loadFailed = $state(false);
	let quickOpen = $state(false);

	function startLoad(): void {
		loadStarted = true;
		loadFailed = false;
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
				loadFailed = true;
			}
		})();
	}

	$effect(() => {
		const redirect = decideAuthRedirect(auth, page.url.pathname);
		if (redirect && redirect !== page.url.pathname) {
			void goto(resolve(redirect));
			return;
		}
		if (auth.status !== 'authenticated' || loadStarted) return;
		startLoad();
	});

	function retryLoad(): void {
		loadStarted = false;
		startLoad();
	}

	function onQuickAdd(): void {
		quickOpen = true;
	}

	async function onQuickSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		try {
			const client = getApiClient();
			if (target.projectId !== null) {
				await projectsApi.createTask(client, target.projectId, payload);
				toast.success('Task added to project');
				void goto(resolve('/(app)/project/[id]', { id: String(target.projectId) }));
				return;
			}
			await tasksApi.createInbox(client, payload);
			toast.success('Task added to inbox');
			void goto(resolve('/(app)/inbox'));
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
	}

	function onKeydown(e: KeyboardEvent): void {
		if (e.metaKey || e.ctrlKey || e.altKey) return;
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

{#if auth.status !== 'authenticated' || (!dataReady && !loadFailed)}
	<div class="flex h-screen items-center justify-center text-sm text-muted-foreground">
		Loading workspace…
	</div>
{:else if loadFailed && !dataReady}
	<div class="flex h-screen flex-col items-center justify-center gap-3 text-sm">
		<p class="text-muted-foreground">Failed to load workspace.</p>
		<button class="rounded-md border px-3 py-1 hover:bg-muted" onclick={retryLoad}>Retry</button>
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
	<QuickAddDialog bind:open={quickOpen} onSubmit={onQuickSubmit} />
{/if}
