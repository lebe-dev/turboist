<script lang="ts">
	import Sidebar from '$lib/components/app/Sidebar.svelte';
	import Topbar from '$lib/components/app/Topbar.svelte';
	import ContextFilterBanner from '$lib/components/app/ContextFilterBanner.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import * as Sheet from '$lib/components/ui/sheet';
	import { sidebarStore } from '$lib/stores/sidebar.svelte';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { decideAuthRedirect } from '$lib/auth/guard';
	import { page } from '$app/state';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { inboxStatsStore } from '$lib/stores/inboxStats.svelte';
	import { pinnedTasksStore } from '$lib/stores/pinnedTasks.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { describeError } from '$lib/utils/taskActions';
	import { dayKeyInTz, shiftDayKey } from '$lib/utils/format';
	import type { TaskInput } from '$lib/api/types';

	let { children } = $props();

	const STATIC_TITLES: Record<string, string> = {
		'/today': 'Today',
		'/tomorrow': 'Tomorrow',
		'/inbox': 'Inbox',
		'/week': 'This week',
		'/backlog': 'Backlog',
		'/next-week': 'Next week',
		'/completed': 'Completed',
		'/search': 'Search',
		'/troiki': 'Troiki',
		'/settings': 'Settings'
	};

	const documentTitle = $derived.by(() => {
		const pageName = STATIC_TITLES[page.url.pathname] ?? viewFilterStore.title;
		return pageName ? `${pageName} — Turboist` : 'Turboist';
	});

	const auth = getAuthStore();

	let dataReady = $state(false);
	let loadStarted = $state(false);
	let loadFailed = $state(false);
	let quickOpen = $state(false);
	let mobileSidebarOpen = $state(false);

	$effect(() => {
		void page.url.pathname;
		mobileSidebarOpen = false;
	});

	function startLoad(): void {
		loadStarted = true;
		loadFailed = false;
		void (async () => {
			try {
				await Promise.all([
					configStore.load(),
					contextsStore.load(),
					projectsStore.load(),
					labelsStore.load(),
					planStatsStore.load(),
					inboxStatsStore.load(),
					pinnedTasksStore.load(),
					userStateStore.load(),
					troikiStore.load()
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

	const quickAddDefaults = $derived.by(() => {
		const path = page.url.pathname;
		const tz = configStore.value?.timezone ?? null;
		const todayKey = dayKeyInTz(new Date(), tz);
		const tomorrowKey = shiftDayKey(todayKey, 1);

		let projectId: number | null = null;
		let contextId: number | null = null;
		let labelIds: number[] = [];
		let dueDate = '';

		if (path === '/today') {
			dueDate = todayKey;
		} else if (path === '/tomorrow') {
			dueDate = tomorrowKey;
		} else if (path.startsWith('/project/')) {
			const id = Number(page.params.id);
			if (Number.isFinite(id)) projectId = id;
		} else if (path.startsWith('/label/')) {
			const id = Number(page.params.id);
			if (Number.isFinite(id)) labelIds = [id];
		} else if (path.startsWith('/context/')) {
			const id = Number(page.params.id);
			if (Number.isFinite(id)) contextId = id;
		}

		return { projectId, contextId, labelIds, dueDate };
	});

	async function onQuickSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		try {
			const client = getApiClient();
			if (target.projectId !== null) {
				const created = await projectsApi.createTask(client, target.projectId, payload);
				toast.success('Task added to project');
				const projectPath = resolve(`/project/${target.projectId}`);
				if (page.url.pathname === projectPath) {
					window.dispatchEvent(
						new CustomEvent('turboist:task-created', {
							detail: { task: created, projectId: target.projectId }
						})
					);
				} else {
					void goto(projectPath);
				}
				return;
			}
			const ctxId = quickAddDefaults.contextId;
			if (ctxId !== null) {
				const created = await contextsApi.createTask(client, ctxId, payload);
				toast.success('Task added to context');
				const contextPath = resolve(`/context/${ctxId}`);
				if (page.url.pathname === contextPath) {
					window.dispatchEvent(
						new CustomEvent('turboist:task-created', {
							detail: { task: created, projectId: null, contextId: ctxId }
						})
					);
				}
				return;
			}
			const created = await tasksApi.createInbox(client, payload);
			toast.success('Task added to inbox');
			void inboxStatsStore.load().catch(() => {});
			const inboxPath = resolve('/inbox');
			if (page.url.pathname === inboxPath) {
				window.dispatchEvent(
					new CustomEvent('turboist:task-created', {
						detail: { task: created, projectId: null }
					})
				);
			} else {
				void goto(inboxPath);
			}
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

<svelte:head>
	<title>{documentTitle}</title>
</svelte:head>

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
		<div
				class="hidden overflow-hidden transition-[width] duration-200 ease-in-out md:flex"
				style:width={sidebarStore.collapsed ? '0' : '16rem'}
			>
				<Sidebar />
			</div>
		<div class="flex min-w-0 flex-1 flex-col">
			<Topbar {onQuickAdd} onMenuClick={() => (mobileSidebarOpen = true)} />
			<ContextFilterBanner />
			<main class="flex-1 overflow-y-auto">
				{@render children()}
			</main>
		</div>
	</div>
	<Sheet.Root bind:open={mobileSidebarOpen}>
		<Sheet.Content
			side="left"
			class="w-64 max-w-[85vw] border-sidebar-border bg-sidebar p-0 sm:max-w-[85vw] md:hidden"
			showCloseButton={false}
		>
			<Sheet.Title class="sr-only">Navigation</Sheet.Title>
			<Sheet.Description class="sr-only">Workspace navigation menu</Sheet.Description>
			<Sidebar />
		</Sheet.Content>
	</Sheet.Root>
	<QuickAddDialog
		bind:open={quickOpen}
		defaultProjectId={quickAddDefaults.projectId}
		defaultLabelIds={quickAddDefaults.labelIds}
		defaultDueDate={quickAddDefaults.dueDate}
		onSubmit={onQuickSubmit}
	/>
{/if}
