import { logger } from '$lib/stores/logger';
import { getAppConfig, patchState } from '$lib/api/client';
import { setBackend, getBackend } from '$lib/api/backend';
import { DefaultBackendConnector } from '$lib/api/default-backend';
import { QueuedBackend } from '$lib/api/queued-backend';
import { actionQueue } from '$lib/sync/action-queue.svelte';
import type { AllFiltersState, AutoLabelMapping, Label, LabelConfig, LabelProjectMapping, Project, ProjectTask, QuickCaptureConfig, View } from '$lib/api/types';
import { applyLocaleFromConfig } from '$lib/i18n';
import { compileAutoLabels, matchAutoLabels } from '$lib/utils/auto-labels';
import { contextsStore } from './contexts.svelte';
import { pinnedStore } from './pinned.svelte';
import { collapsedStore } from './collapsed.svelte';
import { sectionsStore } from './sections.svelte';
import { sidebarStore } from './sidebar.svelte';
import { planningStore } from './planning.svelte';
import { dayPartNotesStore } from './day-part-notes.svelte';
import { tasksStore } from './tasks.svelte';
import { projectTasksStore } from './project-tasks.svelte';
import { wsClient } from '$lib/ws/client.svelte';
import { saveAppConfig, loadAppConfig } from '$lib/sync/db';
import { initState, destroyState, loadPersistedUI } from '$lib/state/index.svelte';

const LOCAL_STORAGE_KEYS = [
	'turboist:context',
	'turboist:view',
	'turboist:pinned-tasks',
	'turboist:collapsed',
	'turboist:sidebar-collapsed',
	'turboist:planning',
	'turboist:locale',
	'turboist:all-filters'
] as const;

// One-time migration: push any localStorage state to the server and clear it.
async function migrateLocalStorage(): Promise<void> {
	try {
		const hasAny = LOCAL_STORAGE_KEYS.some((k) => localStorage.getItem(k) !== null);
		if (!hasAny) return;

		const update: Record<string, unknown> = {};

		const ctx = localStorage.getItem('turboist:context');
		if (ctx) update.active_context_id = ctx;

		const view = localStorage.getItem('turboist:view');
		if (view) update.active_view = view;

		const pinned = localStorage.getItem('turboist:pinned-tasks');
		if (pinned) {
			try {
				update.pinned_tasks = JSON.parse(pinned);
			} catch {
				// ignore
			}
		}

		const collapsed = localStorage.getItem('turboist:collapsed');
		if (collapsed) {
			try {
				update.collapsed_ids = JSON.parse(collapsed);
			} catch {
				// ignore
			}
		}

		const sidebar = localStorage.getItem('turboist:sidebar-collapsed');
		if (sidebar) update.sidebar_collapsed = sidebar === 'true';

		const planning = localStorage.getItem('turboist:planning');
		if (planning) update.planning_open = planning === 'true';

		const loc = localStorage.getItem('turboist:locale');
		if (loc) update.locale = loc;

		const allFiltersRaw = localStorage.getItem('turboist:all-filters');
		if (allFiltersRaw) {
			try {
				const parsed = JSON.parse(allFiltersRaw);
				update.all_filters = {
					selected_priorities: parsed.selectedPriorities ?? [],
					selected_labels: parsed.selectedLabels ?? [],
					links_only: parsed.linksOnly ?? false,
					filters_expanded: parsed.filtersExpanded ?? false
				};
			} catch {
				// ignore
			}
		}

		if (Object.keys(update).length > 0) {
			await patchState(update as Parameters<typeof patchState>[0]);
		}

		// Clear old keys
		for (const key of LOCAL_STORAGE_KEYS) {
			localStorage.removeItem(key);
		}
	} catch {
		// Migration is best-effort
	}
}

function createAppStore() {
	let initialized = $state(false);
	let labels = $state<Label[]>([]);
	let labelConfigs = $state<LabelConfig[]>([]);
	let quickCapture = $state<QuickCaptureConfig | null>(null);
	let projectTasks = $state<ProjectTask[]>([]);
	let autoLabelMappings = $state<AutoLabelMapping[]>([]);
	let _compiledAutoLabels = $derived(compileAutoLabels(autoLabelMappings));
	let labelProjectMap = $state<LabelProjectMapping[]>([]);
	let labelProjectMapEnabled = $state(false);
	let _projects = $state<Project[]>([]);
	let inboxProjectId = $state<string>('');
	let quickCaptureOpen = $state(false);
	let allFilters = $state<AllFiltersState | null>(null);
	let autoRemovePaused = $state(false);

	function hydrateFromConfig(cfg: import('$lib/api/types').AppConfig): void {
		labels = cfg.labels;
		labelConfigs = cfg.label_configs ?? [];
		quickCapture = cfg.quick_capture;
		projectTasks = cfg.project_tasks ?? [];
		autoLabelMappings = cfg.auto_labels ?? [];
		labelProjectMap = cfg.label_project_map?.mappings ?? [];
		labelProjectMapEnabled = cfg.label_project_map?.enabled ?? false;
		_projects = cfg.projects.map((p) => ({ id: p.id, name: p.name, color: p.color, sections: p.sections }));
		inboxProjectId = cfg.settings.inbox_project_id ?? '';

		contextsStore.init(
			cfg.contexts,
			cfg.state.active_context_id,
			cfg.state.active_view as View
		);
		pinnedStore.init(cfg.state.pinned_tasks, cfg.settings.max_pinned);
		const cachedUI = loadPersistedUI();
		const hasCachedUI = cachedUI !== null;
		collapsedStore.init(hasCachedUI ? cfg.state.collapsed_ids : []);
		sectionsStore.init(
			cachedUI?.collapsed_section_ids ?? [],
			cachedUI?.pinned_section_ids ?? []
		);
		sidebarStore.init(cfg.state.sidebar_collapsed);
		planningStore.initActive(cfg.state.planning_open);
		dayPartNotesStore.init(
			cfg.state.day_part_notes ?? {},
			cfg.settings.max_day_part_note_length ?? 200
		);

		applyLocaleFromConfig(cfg.state.locale);
		allFilters = cfg.state.all_filters ?? null;
		autoRemovePaused = cfg.auto_remove?.paused ?? false;
	}

	async function init(): Promise<void> {
		logger.log('app', 'init start');

		// Set up the backend connector chain: Default → Queued
		const defaultBackend = new DefaultBackendConnector();
		setBackend(new QueuedBackend(defaultBackend, actionQueue));

		// Load any pending offline actions from previous session
		await actionQueue.init();

		// Migrate localStorage first (one-time)
		await migrateLocalStorage();

		// Initialize SyncroState + y-indexeddb (loads cached data from IDB)
		await initState();
		logger.log('app', 'SyncroState initialized, y-indexeddb synced');

		let cfg;
		try {
			cfg = await getAppConfig();
			logger.log('app', 'config loaded from API');
			// Cache config to IDB for offline use
			saveAppConfig(cfg).catch((e) => logger.error('app', String(e)));
		} catch {
			logger.warn('app', 'config API failed, trying IDB cache');
			// Fallback to cached config from IDB
			cfg = await loadAppConfig();
			if (!cfg) throw new Error('No network and no cached config');
			logger.log('app', 'config loaded from IDB cache');
		}

		hydrateFromConfig(cfg);

		// Connect WebSocket
		wsClient.connect();

		// Start task store (registers WS handlers and subscribes)
		await tasksStore.start();

		// Load all tasks for project views (offline-first via Y.Doc)
		projectTasksStore.start();

		// Start auto-flush timer using sync_interval from config (default 60s)
		const syncMs = (cfg.settings.sync_interval || 60) * 1000;
		actionQueue.startAutoFlush(defaultBackend, syncMs);

		// Flush any queued actions from previous session immediately
		actionQueue.flushNow().catch((e) => logger.error('app', `Initial queue flush failed: ${e}`));

		initialized = true;
		logger.log('app', 'init complete');
	}

	function destroy(): void {
		actionQueue.stopAutoFlush();
		tasksStore.stop();
		projectTasksStore.stop();
		wsClient.disconnect();
		destroyState();
		initialized = false;
	}

	function shouldInheritToSubtasks(labelName: string): boolean {
		const cfg = labelConfigs.find((lc) => lc.name === labelName);
		if (!cfg) return true;
		return cfg.inherit_to_subtasks;
	}

	function getMatchingAutoLabels(title: string): string[] {
		return matchAutoLabels(title, _compiledAutoLabels);
	}

	function resolveProjectIdForLabels(labels: string[]): string | null {
		if (!labelProjectMapEnabled || labelProjectMap.length === 0) return null;
		for (const mapping of labelProjectMap) {
			if (labels.includes(mapping.label)) {
				const proj = _projects.find((p) => p.name === mapping.project);
				if (proj) return proj.id;
			}
		}
		return inboxProjectId || null;
	}

	return {
		get initialized() {
			return initialized;
		},
		get labels() {
			return labels;
		},
		get labelConfigs() {
			return labelConfigs;
		},
		get quickCapture() {
			return quickCapture;
		},
		get projectTasks() {
			return projectTasks;
		},
		get projects() {
			return _projects;
		},
		get compiledAutoLabels() {
			return _compiledAutoLabels;
		},
		get labelProjectMap() {
			return labelProjectMap;
		},
		get inboxProjectId() {
			return inboxProjectId;
		},
		get quickCaptureOpen() {
			return quickCaptureOpen;
		},
		set quickCaptureOpen(v: boolean) {
			quickCaptureOpen = v;
		},
		get allFilters() {
			return allFilters;
		},
		get autoRemovePaused() {
			return autoRemovePaused;
		},
		saveAllFilters(f: AllFiltersState) {
			allFilters = f;
			patchState({ all_filters: f }).catch(console.error);
		},
		shouldInheritToSubtasks,
		getMatchingAutoLabels,
		resolveProjectIdForLabels,
		init,
		destroy
	};
}

export const appStore = createAppStore();
