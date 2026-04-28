<script lang="ts">
	import Sidebar from '$lib/components/app/Sidebar.svelte';
	import Topbar from '$lib/components/app/Topbar.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import { getAuthStore } from '$lib/auth/store.svelte';
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
	let quickOpen = $state(false);

	$effect(() => {
		if (auth.status === 'guest') {
			void goto(resolve('/login'));
			return;
		}
		if (auth.status !== 'authenticated' || loadStarted) return;
		loadStarted = true;
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
				loadStarted = false;
			}
		})();
	});

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
				return;
			}
			await tasksApi.createInbox(client, payload);
			toast.success('Task added to inbox');
		} catch (err) {
			toast.error(describeError(err, 'Failed to add task'));
		}
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
	<QuickAddDialog bind:open={quickOpen} onSubmit={onQuickSubmit} />
{/if}
