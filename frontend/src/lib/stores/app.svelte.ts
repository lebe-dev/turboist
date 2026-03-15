import { getAppConfig, patchState } from '$lib/api/client';
import type { Label, QuickCaptureConfig, View } from '$lib/api/types';
import { contextsStore } from './contexts.svelte';
import { pinnedStore } from './pinned.svelte';
import { collapsedStore } from './collapsed.svelte';
import { sidebarStore } from './sidebar.svelte';
import { planningStore } from './planning.svelte';
import { tasksStore } from './tasks.svelte';

const LOCAL_STORAGE_KEYS = [
	'turboist:context',
	'turboist:view',
	'turboist:pinned-tasks',
	'turboist:collapsed',
	'turboist:sidebar-collapsed',
	'turboist:planning'
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
	let quickCapture = $state<QuickCaptureConfig | null>(null);

	async function init(): Promise<void> {
		// Migrate localStorage first (one-time)
		await migrateLocalStorage();

		const cfg = await getAppConfig();

		// Store shared data
		labels = cfg.labels;
		quickCapture = cfg.quick_capture;

		// Init all stores from server state
		contextsStore.init(
			cfg.contexts,
			cfg.state.active_context_id,
			cfg.state.active_view as View
		);
		pinnedStore.init(cfg.state.pinned_tasks, cfg.settings.max_pinned);
		collapsedStore.init(cfg.state.collapsed_ids);
		sidebarStore.init(cfg.state.sidebar_collapsed);
		planningStore.initActive(cfg.state.planning_open);

		// Start task polling with the configured interval
		await tasksStore.start(cfg.settings.poll_interval);

		initialized = true;
	}

	return {
		get initialized() {
			return initialized;
		},
		get labels() {
			return labels;
		},
		get quickCapture() {
			return quickCapture;
		},
		init
	};
}

export const appStore = createAppStore();
