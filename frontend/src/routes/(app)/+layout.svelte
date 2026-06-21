<script lang="ts">
	import Sidebar from '$lib/components/app/Sidebar.svelte';
	import Topbar from '$lib/components/app/Topbar.svelte';
	import ContextFilterBanner from '$lib/components/app/ContextFilterBanner.svelte';
	import TodayBanner from '$lib/components/app/TodayBanner.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import SelectionActionBar from '$lib/components/task/SelectionActionBar.svelte';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import type { DayPart, Priority, Task } from '$lib/api/types';
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
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import { currentTaskStore } from '$lib/stores/currentTask.svelte';
	import { harpoonStore } from '$lib/stores/harpoon.svelte';
	import { templatesStore } from '$lib/stores/templates.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { contexts as contextsApi } from '$lib/api/endpoints/contexts';
	import { templates as templatesApi } from '$lib/api/endpoints/templates';
	import ProjectPickerDialog from '$lib/components/dialog/ProjectPickerDialog.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { shiftDayKey } from '$lib/utils/format';
	import { nowStore } from '$lib/stores/now.svelte';
	import type { TaskInput } from '$lib/api/types';
	import { t, setLocale, isSupportedLocale } from '$lib/i18n';
	import { eventsClient, type EventScope } from '$lib/realtime/events.svelte';

	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import CardsIcon from 'phosphor-svelte/lib/Cards';

	let { children } = $props();

	const TITLE_KEYS: Record<string, string> = {
		'/today': 'nav.today',
		'/tomorrow': 'nav.tomorrow',
		'/inbox': 'nav.inbox',
		'/week': 'nav.thisWeek',
		'/next-week': 'nav.nextWeek',
		'/completed': 'nav.completed',
		'/search': 'nav.search',
		'/troiki': 'nav.troiki',
		'/settings': 'nav.settings'
	};

	const documentTitle = $derived.by(() => {
		const key = TITLE_KEYS[page.url.pathname];
		const pageName = key ? $t(key) : viewFilterStore.title;
		const appName = $t('app.name');
		return pageName ? `${pageName} — ${appName}` : appName;
	});

	const auth = getAuthStore();

	const quickAddHidden = $derived(page.url.pathname === '/settings');

	let dataReady = $state(false);
	let loadStarted = $state(false);
	let loadFailed = $state(false);
	let quickOpen = $state(false);
	let mobileSidebarOpen = $state(false);
	let templateSheetOpen = $state(false);
	let projectPickerOpen = $state(false);
	let pendingTemplateId = $state<number | null>(null);
	let followUpOverride = $state<{
		projectId: number | null;
		labelIds: number[];
		priority: Priority;
		dayPart: DayPart;
		parentId: number | null;
		sectionId: number | null;
	} | null>(null);
	let quickAddPrefill = $state<{
		title: string;
		description: string;
		dueDate: string;
		dayPart: DayPart;
	} | null>(null);
	let groupOpen = $state(false);
	let groupBusy = $state(false);
	let groupSnapshot = $state<{
		tasks: Array<{ id: number; title: string }>;
		warning: string | null;
		defaultProjectId: number | null;
		defaultContextId: number | null;
		defaultSectionId: number | null;
		childIds: number[];
	} | null>(null);

	$effect(() => {
		void page.url.pathname;
		mobileSidebarOpen = false;
		taskSelectionStore.disable();
	});

	$effect(() => {
		if (!quickOpen) {
			followUpOverride = null;
			quickAddPrefill = null;
		}
	});

	$effect(() => {
		function onVisible(): void {
			if (document.visibilityState === 'visible') nowStore.refresh();
		}
		function onFocus(): void {
			nowStore.refresh();
		}
		function onPageShow(): void {
			nowStore.refresh();
		}

		nowStore.scheduleMidnight();
		document.addEventListener('visibilitychange', onVisible);
		window.addEventListener('focus', onFocus);
		window.addEventListener('pageshow', onPageShow);
		return () => {
			document.removeEventListener('visibilitychange', onVisible);
			window.removeEventListener('focus', onFocus);
			window.removeEventListener('pageshow', onPageShow);
			nowStore.teardown();
		};
	});

	function startLoad(): void {
		loadStarted = true;
		loadFailed = false;
		void (async () => {
			try {
				// configStore.load() pulls everything the workspace needs in one
				// round-trip and fans it out to the per-domain stores: contexts,
				// projects, labels, settings, appSettings, userState, troiki,
				// planStats, inboxStats and pinnedTasks. Per-domain stores keep
				// their own load() / SSE invalidation channels for refreshes.
				await configStore.load();
				if (isSupportedLocale(settingsStore.locale)) {
					setLocale(settingsStore.locale);
				}
				dataReady = true;
				// Harpoon is a small navigation convenience loaded out-of-band: a
				// failure here must not block the workspace, so it is fire-and-forget.
				void harpoonStore.load().catch(() => {});
				// Templates power the quick-add "from template" menu and the
				// settings editor; load out-of-band so a failure never blocks
				// the workspace.
				void templatesStore.load().catch(() => {});
			} catch (err) {
				const message = err instanceof Error ? err.message : $t('app.workspaceFailed');
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

	// Real-time invalidation channel. Started after the user is authenticated
	// and the initial workspace load has finished — so that handlers for shell
	// stores (which we re-load below) do not race the initial fetch. Page-level
	// views subscribe via `useInvalidation` and receive their own scope events.
	$effect(() => {
		if (auth.status !== 'authenticated' || !dataReady) return;
		eventsClient.start();

		const dispatch = (scope: EventScope): void => {
			window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope } }));
		};

		const unsubs = [
			eventsClient.on('contexts', () => {
				void contextsStore.load();
				dispatch('contexts');
			}),
			eventsClient.on('labels', () => {
				void labelsStore.load();
				dispatch('labels');
			}),
			eventsClient.on('projects', () => {
				void projectsStore.load();
				dispatch('projects');
			}),
			eventsClient.on('inbox', () => {
				void inboxStatsStore.load();
				dispatch('inbox');
			}),
			eventsClient.on('plan', () => {
				void planStatsStore.load();
				dispatch('plan');
			}),
			eventsClient.on('tasks', () => {
				void pinnedTasksStore.load();
				dispatch('tasks');
			}),
			eventsClient.on('calendar', () => dispatch('calendar')),
			eventsClient.on('sections', () => dispatch('sections'))
		];
		return () => {
			for (const u of unsubs) u();
		};
	});

	// On SSE reconnect after a drop (e.g., the tab was suspended), the server
	// has no replay — so refresh everything once and notify page-level views
	// to revalidate.
	let lastReconnect = $state<number | null>(null);
	$effect(() => {
		const at = eventsClient.reconnectedAt;
		if (at === null || at === lastReconnect) return;
		lastReconnect = at;
		void Promise.all([
			contextsStore.load(),
			labelsStore.load(),
			projectsStore.load(),
			inboxStatsStore.load(),
			planStatsStore.load(),
			pinnedTasksStore.load()
		]);
		const scopes: EventScope[] = [
			'tasks',
			'calendar',
			'inbox',
			'projects',
			'labels',
			'contexts',
			'sections',
			'plan'
		];
		for (const scope of scopes) {
			window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope } }));
		}
	});

	$effect(() => {
		if (auth.status !== 'authenticated') {
			eventsClient.stop();
		}
	});

	function retryLoad(): void {
		loadStarted = false;
		startLoad();
	}

	function onQuickAdd(): void {
		followUpOverride = null;
		quickAddPrefill = null;
		quickOpen = true;
	}

	$effect(() => {
		const handler = (e: Event) => {
			const detail = (e as CustomEvent<{ title?: string; description?: string; dueDate?: string; dayPart?: DayPart }>).detail;
			quickAddPrefill = {
				title: detail?.title ?? '',
				description: detail?.description ?? '',
				dueDate: detail?.dueDate ?? '',
				dayPart: detail?.dayPart ?? 'none'
			};
			followUpOverride = null;
			quickOpen = true;
		};
		window.addEventListener('turboist:quick-add', handler);
		return () => window.removeEventListener('turboist:quick-add', handler);
	});

	async function onGroupRequest(): Promise<void> {
		const ids = Array.from(taskSelectionStore.ids);
		if (ids.length < 2) return;
		groupBusy = true;
		try {
			const client = getApiClient();
			const fetched = await Promise.all(ids.map((id) => tasksApi.get(client, id)));
			const projectIds = new Set(fetched.map((t) => t.projectId));
			const sectionIds = new Set(fetched.map((t) => t.sectionId));
			const contextIds = new Set(fetched.map((t) => t.contextId));
			const sameScope = projectIds.size === 1 && sectionIds.size === 1 && contextIds.size === 1;
			const first = fetched[0];
			groupSnapshot = {
				tasks: fetched.map((t) => ({ id: t.id, title: t.title })),
				warning: sameScope ? null : $t('dialog.quickAdd.wrap.warningMixed'),
				defaultProjectId: sameScope ? first.projectId : null,
				defaultContextId: sameScope ? first.contextId : null,
				defaultSectionId: sameScope ? first.sectionId : null,
				childIds: ids
			};
			groupOpen = true;
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedGroup')));
		} finally {
			groupBusy = false;
		}
	}

	async function onGroupSubmit(
		payload: TaskInput,
		target: { projectId: number | null; sectionId: number | null }
	): Promise<void> {
		if (!groupSnapshot) return;
		try {
			const client = getApiClient();
			let contextId = groupSnapshot.defaultContextId;
			if (target.projectId !== null) {
				const project = projectsStore.items.find((p) => p.id === target.projectId);
				if (project) contextId = project.contextId;
			}
			const result = await tasksApi.group(client, {
				...payload,
				projectId: target.projectId,
				sectionId: target.sectionId,
				contextId,
				childIds: groupSnapshot.childIds
			});
			const failedCount = result.failed.length;
			if (failedCount > 0) {
				toast.error(
					$t('task.toast.groupedPartial', {
						values: { ok: result.succeeded.length, failed: failedCount }
					})
				);
			} else {
				toast.success(
					$t('task.toast.grouped', { values: { count: result.succeeded.length } })
				);
			}
			taskSelectionStore.disable();
			window.dispatchEvent(
				new CustomEvent('turboist:task-created', {
					detail: {
						task: result.parent,
						projectId: result.parent.projectId,
						contextId: result.parent.contextId
					}
				})
			);
			window.dispatchEvent(
				new CustomEvent('turboist:tasks-grouped', {
					detail: {
						parent: result.parent,
						childIds: result.succeeded
					}
				})
			);
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedGroup')));
			throw err;
		}
	}

	function onFollowUpNext(task: Task): void {
		followUpOverride = {
			projectId: task.projectId,
			labelIds: task.labels.map((l) => l.id),
			priority: task.priority,
			dayPart: task.dayPart,
			parentId: task.parentId,
			sectionId: task.sectionId
		};
		quickOpen = true;
	}

	$effect(() => {
		const handler = (e: Event) => {
			const detail = (e as CustomEvent<{ task: Task }>).detail;
			if (detail?.task) onFollowUpNext(detail.task);
		};
		window.addEventListener('turboist:followUpNext', handler);
		return () => window.removeEventListener('turboist:followUpNext', handler);
	});

	const quickAddDefaults = $derived.by(() => {
		const path = page.url.pathname;
		const todayKey = nowStore.todayKey;
		const tomorrowKey = shiftDayKey(todayKey, 1);

		let projectId: number | null = null;
		let contextId: number | null = null;
		let labelIds: number[] = [];
		let dueDate = '';

		if (path === '/today') {
			dueDate = todayKey;
		} else if (path === '/tomorrow') {
			dueDate = tomorrowKey;
		} else if (path.startsWith('/task/')) {
			projectId = currentTaskStore.projectId;
			labelIds = currentTaskStore.labelIds;
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

	async function applySectionMove(
		client: ReturnType<typeof getApiClient>,
		taskId: number,
		contextId: number | null,
		projectId: number,
		sectionId: number | null
	): Promise<void> {
		if (sectionId === null || contextId === null) return;
		try {
			await tasksApi.move(client, taskId, { contextId, projectId, sectionId });
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedSetSection')));
		}
	}

	async function onQuickSubmit(
		payload: TaskInput,
		target: {
			projectId: number | null;
			labels: string[];
			parentId: number | null;
			sectionId: number | null;
		}
	): Promise<void> {
		try {
			const client = getApiClient();
			if (target.parentId !== null) {
				const created = await tasksApi.createSubtask(client, target.parentId, payload);
				toast.success($t('task.toast.subtaskAdded'));
				window.dispatchEvent(
					new CustomEvent('turboist:task-created', {
						detail: {
							task: created,
							projectId: created.projectId,
							contextId: created.contextId
						}
					})
				);
				return;
			}
			if (target.projectId !== null) {
				const created = await projectsApi.createTask(client, target.projectId, payload);
				await applySectionMove(
					client,
					created.id,
					created.contextId,
					target.projectId,
					target.sectionId
				);
				toast.success($t('task.toast.addedToProject'));
				window.dispatchEvent(
					new CustomEvent('turboist:task-created', {
						detail: {
							task: created,
							projectId: created.projectId,
							contextId: created.contextId
						}
					})
				);
				return;
			}
			const ctxId = quickAddDefaults.contextId;
			if (ctxId !== null) {
				const created = await contextsApi.createTask(client, ctxId, payload);
				toast.success($t('task.toast.addedToContext'));
				window.dispatchEvent(
					new CustomEvent('turboist:task-created', {
						detail: {
							task: created,
							projectId: created.projectId,
							contextId: created.contextId
						}
					})
				);
				return;
			}
			const created = await tasksApi.createInbox(client, payload);
			toast.success($t('task.toast.addedToInbox'));
			void inboxStatsStore.load().catch(() => {});
			window.dispatchEvent(
				new CustomEvent('turboist:task-created', {
					detail: {
						task: created,
						projectId: created.projectId,
						contextId: created.contextId
					}
				})
			);
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedAdd')));
		}
	}

	async function instantiateTemplate(templateId: number, projectId: number): Promise<void> {
		try {
			const client = getApiClient();
			const result = await templatesApi.instantiate(client, templateId, projectId);
			toast.success(
				$t('template.toast.created', { values: { count: result.subtasks.length + 1 } })
			);
			// Announce the root AND each subtask so optimistic list views render the
			// whole tree immediately. The SSE echo for this tab is suppressed
			// (self-origin), so without per-subtask events the children would only
			// appear after a manual refresh.
			for (const task of [result.root, ...result.subtasks]) {
				window.dispatchEvent(
					new CustomEvent('turboist:task-created', {
						detail: {
							task,
							projectId: task.projectId,
							contextId: task.contextId
						}
					})
				);
			}
		} catch (err) {
			toast.error(describeError(err, $t('template.toast.failed')));
		}
	}

	// Materialize a template into the current project. When the active route has
	// no project context (e.g. /today, a context view), ask the user to pick one.
	function onPickTemplate(templateId: number): void {
		templateSheetOpen = false;
		const projectId = quickAddDefaults.projectId;
		if (projectId !== null) {
			void instantiateTemplate(templateId, projectId);
			return;
		}
		pendingTemplateId = templateId;
		projectPickerOpen = true;
	}

	function onProjectPicked(projectId: number): void {
		const templateId = pendingTemplateId;
		pendingTemplateId = null;
		if (templateId !== null) void instantiateTemplate(templateId, projectId);
	}

	// Mobile FAB: a tap opens quick-add; a long-press opens the template sheet.
	let fabLongPressTimer: ReturnType<typeof setTimeout> | null = null;
	let fabLongPressed = false;

	function fabPointerDown(): void {
		fabLongPressed = false;
		fabLongPressTimer = setTimeout(() => {
			fabLongPressed = true;
			templateSheetOpen = true;
		}, 450);
	}

	function fabPointerCancel(): void {
		if (fabLongPressTimer !== null) {
			clearTimeout(fabLongPressTimer);
			fabLongPressTimer = null;
		}
	}

	function fabClick(): void {
		fabPointerCancel();
		if (fabLongPressed) {
			fabLongPressed = false;
			return;
		}
		onQuickAdd();
	}

	function onKeydown(e: KeyboardEvent): void {
		if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && (e.key === 'j' || e.key === 'J')) {
			e.preventDefault();
			sidebarStore.toggle();
			return;
		}
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
		{$t('app.loadingWorkspace')}
	</div>
{:else if loadFailed && !dataReady}
	<div class="flex h-screen flex-col items-center justify-center gap-3 text-sm">
		<p class="text-muted-foreground">{$t('app.workspaceFailed')}</p>
		<button class="rounded-md border px-3 py-1 hover:bg-muted" onclick={retryLoad}>{$t('app.retry')}</button>
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
			<Topbar
				onQuickAdd={quickAddHidden ? undefined : onQuickAdd}
				onPickTemplate={quickAddHidden ? undefined : onPickTemplate}
				onMenuClick={() => (mobileSidebarOpen = true)}
			/>
			{#if !page.url.pathname.startsWith('/settings')}
				<ContextFilterBanner />
			{/if}
			{#if page.url.pathname === '/today'}
				<TodayBanner />
			{/if}
			<main class="flex-1 overflow-x-hidden overflow-y-auto {quickAddHidden ? '' : 'pb-24 md:pb-0'}">
				{@render children()}
			</main>
		</div>
	</div>
	<Sheet.Root bind:open={mobileSidebarOpen}>
		<Sheet.Content
			side="left"
			class="w-[82vw] border-sidebar-border bg-sidebar p-0 md:hidden"
			showCloseButton={false}
		>
			<Sheet.Title class="sr-only">{$t('sidebar.navigation')}</Sheet.Title>
			<Sheet.Description class="sr-only">{$t('sidebar.navigationDesc')}</Sheet.Description>
			<Sidebar />
		</Sheet.Content>
	</Sheet.Root>
	<QuickAddDialog
		bind:open={quickOpen}
		defaultTitle={quickAddPrefill?.title ?? ''}
		defaultDescription={followUpOverride ? '' : (quickAddPrefill?.description ?? '')}
		defaultProjectId={followUpOverride ? followUpOverride.projectId : quickAddDefaults.projectId}
		defaultLabelIds={followUpOverride ? followUpOverride.labelIds : quickAddDefaults.labelIds}
		defaultDueDate={followUpOverride ? '' : quickAddPrefill?.dueDate ?? quickAddDefaults.dueDate}
		defaultPriority={followUpOverride?.priority ?? 'no-priority'}
		defaultDayPart={followUpOverride?.dayPart ?? quickAddPrefill?.dayPart ?? 'none'}
		defaultParentId={followUpOverride?.parentId ?? null}
		defaultSectionId={followUpOverride?.sectionId ?? null}
		onSubmit={onQuickSubmit}
	/>
	{#if groupSnapshot}
		<QuickAddDialog
			bind:open={groupOpen}
			defaultProjectId={groupSnapshot.defaultProjectId}
			defaultSectionId={groupSnapshot.defaultSectionId}
			wrap={{ tasks: groupSnapshot.tasks, warning: groupSnapshot.warning }}
			onSubmit={onGroupSubmit}
		/>
	{/if}
	<SelectionActionBar onGroup={onGroupRequest} busy={groupBusy} />
	{#if !quickAddHidden && !taskSelectionStore.mode}
		<button
			onclick={fabClick}
			onpointerdown={fabPointerDown}
			onpointerup={fabPointerCancel}
			onpointerleave={fabPointerCancel}
			oncontextmenu={(e) => e.preventDefault()}
			class="fixed bottom-[calc(1.5rem+env(safe-area-inset-bottom))] right-6 z-50 flex h-14 w-14 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg active:scale-95 transition-transform md:hidden"
			aria-label={$t('task.quickAdd')}
		>
			<PlusIcon class="h-7 w-7" />
		</button>
	{/if}
	<Sheet.Root bind:open={templateSheetOpen}>
		<Sheet.Content side="bottom" class="max-h-[70vh] overflow-y-auto rounded-t-lg p-3 md:hidden">
			<Sheet.Header class="px-2 pb-1 pt-0">
				<Sheet.Title>{$t('topbar.quickAddMenu')}</Sheet.Title>
			</Sheet.Header>
			<div class="flex flex-col gap-1 pb-4 pt-2">
				<button
					type="button"
					onclick={() => {
						templateSheetOpen = false;
						onQuickAdd();
					}}
					class="flex items-center gap-3 rounded-md px-3 py-3 text-left text-sm transition-colors hover:bg-accent"
				>
					<PlusIcon class="size-4" />
					{$t('topbar.newTask')}
				</button>
				{#if templatesStore.items.length > 0}
					<div class="mt-1 flex items-center gap-2 px-3 pb-1 text-xs font-medium text-muted-foreground">
						<CardsIcon class="size-3.5" />
						{$t('topbar.fromTemplate')}
					</div>
					{#each templatesStore.items as template (template.id)}
						<button
							type="button"
							onclick={() => onPickTemplate(template.id)}
							class="flex items-center gap-3 rounded-md px-3 py-3 text-left text-sm transition-colors hover:bg-accent"
						>
							<span class="flex-1 truncate">{template.name}</span>
						</button>
					{/each}
				{:else}
					<p class="px-3 py-2 text-xs text-muted-foreground">{$t('settings.templates.empty')}</p>
				{/if}
			</div>
		</Sheet.Content>
	</Sheet.Root>
	<ProjectPickerDialog bind:open={projectPickerOpen} onSelect={onProjectPicked} />
{/if}
