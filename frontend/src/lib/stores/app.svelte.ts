import { logger } from '$lib/stores/logger';
import { getAppConfig, patchState } from '$lib/api/client';
import { setBackend, getBackend } from '$lib/api/backend';
import { DefaultBackendConnector } from '$lib/api/default-backend';
import { QueuedBackend } from '$lib/api/queued-backend';
import { actionQueue } from '$lib/sync/action-queue.svelte';
import type { Label, LabelConfig, QuickCaptureConfig, View } from '$lib/api/types';
import { contextsStore } from './contexts.svelte';
import { pinnedStore } from './pinned.svelte';
import { collapsedStore } from './collapsed.svelte';
import { sidebarStore } from './sidebar.svelte';
import { planningStore } from './planning.svelte';
import { tasksStore } from './tasks.svelte';
import { wsClient } from '$lib/ws/client.svelte';
import { saveAppConfig, loadAppConfig } from '$lib/sync/db';

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
	let labelConfigs = $state<LabelConfig[]>([]);
	let quickCapture = $state<QuickCaptureConfig | null>(null);

	function hydrateFromConfig(cfg: import('$lib/api/types').AppConfig): void {
		labels = cfg.labels;
		labelConfigs = cfg.label_configs ?? [];
		quickCapture = cfg.quick_capture;

		contextsStore.init(
			cfg.contexts,
			cfg.state.active_context_id,
			cfg.state.active_view as View
		);
		pinnedStore.init(cfg.state.pinned_tasks, cfg.settings.max_pinned);
		collapsedStore.init(cfg.state.collapsed_ids);
		sidebarStore.init(cfg.state.sidebar_collapsed);
		planningStore.initActive(cfg.state.planning_open);
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
		wsClient.disconnect();
		initialized = false;
	}

	function shouldInheritToSubtasks(labelName: string): boolean {
		const cfg = labelConfigs.find((lc) => lc.name === labelName);
		if (!cfg) return true;
		return cfg.inherit_to_subtasks;
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
		shouldInheritToSubtasks,
		init,
		destroy
	};
}

export const appStore = createAppStore();
