import { config as configApi } from '../api/endpoints/config';
import { getApiClient, NOT_MODIFIED } from '../api/client';
import type { ConfigResponse } from '../api/types';
import { contextsStore } from './contexts.svelte';
import { projectsStore } from './projects.svelte';
import { labelsStore } from './labels.svelte';
import { settingsStore } from './settings.svelte';
import { appSettingsStore } from './appSettings.svelte';
import { userStateStore } from './userState.svelte';
import { troikiStore } from './troiki.svelte';
import { planStatsStore } from './planStats.svelte';
import { inboxStatsStore } from './inboxStats.svelte';
import { pinnedTasksStore } from './pinnedTasks.svelte';
import { harpoonStore } from './harpoon.svelte';
import { templatesStore } from './templates.svelte';

class ConfigStore {
	value = $state<ConfigResponse | null>(null);

	// load fetches the workspace bootstrap in one round-trip and fans the
	// embedded sections out to the per-domain stores. Replaces the previous
	// eight parallel GETs (contexts, projects, labels, settings, app-settings,
	// state, troiki, config) issued from `+layout.svelte` on every page load,
	// plus the harpoon and task-template GETs that used to follow it.
	//
	// This is the BOOT path: it also writes the client-owned slices (settings,
	// appSettings, userState). Mid-session resyncs must go through `refresh()`
	// instead — see the clobbering note there.
	async load(): Promise<ConfigResponse> {
		const cfg = await configApi.get(getApiClient());
		this.value = cfg;
		contextsStore.setItems(cfg.contexts);
		projectsStore.setItems(cfg.projects);
		labelsStore.setItems(cfg.labels);
		settingsStore.setValue(cfg.settings);
		appSettingsStore.setValue(cfg.appSettings);
		userStateStore.setValue(cfg.userState);
		troikiStore.setValue(cfg.troiki);
		planStatsStore.setValue(cfg.planStats);
		inboxStatsStore.set(cfg.inboxStats.count, cfg.inboxStats.warnThresholdExceeded);
		pinnedTasksStore.setItems(cfg.pinnedTasks);
		// `?? []` guards the offline-first boot after an upgrade, when IndexedDB
		// may still hold a pre-v1.15 /api/v1/config payload without these keys.
		// The db.ts SCHEMA_VERSION bump is the primary defence; this is a belt.
		harpoonStore.setSlots(cfg.harpoon?.slots ?? []);
		templatesStore.setItems(cfg.taskTemplates ?? []);
		return cfg;
	}

	// refresh re-pulls the same aggregate mid-session — after an SSE reconnect,
	// after the outbox drains, or on a burst of remote invalidations — in ONE
	// round-trip instead of the six per-store GETs the shell used to fan out.
	//
	// It deliberately fans out only the SERVER-owned slices. `settings` and
	// `appSettings` are edited exclusively from the settings page through
	// optimistic PATCHes, have no SSE scope of their own, and re-applying the
	// server copy here would revert a toggle whose PATCH is still in flight —
	// for `locale` it would also desync the store from the i18n runtime, since
	// nothing re-invokes setLocale(). They are reconciled on boot only.
	//
	// `userState` IS refreshed (activeContextId should follow the user across
	// devices) but goes through reconcileFromServer, which stands down while a
	// local write is unacknowledged.
	//
	// Returns null when the server reports the workspace is unchanged (304), in
	// which case nothing is applied at all — no parse, no fan-out, no re-render.
	// That is the common case on a phone: every unlock reconnects the SSE stream
	// and triggers a catch-up refresh, and for a single-user app most of those
	// find nothing new.
	async refresh(): Promise<ConfigResponse | null> {
		// Only ask "has it changed?" when we have something to fall back on; a 304
		// against an empty store would leave the shell with no data.
		const cfg = this.value
			? await configApi.getIfChanged(getApiClient())
			: await configApi.get(getApiClient());
		if (cfg === NOT_MODIFIED) return null;
		this.value = cfg;
		contextsStore.setItems(cfg.contexts);
		projectsStore.setItems(cfg.projects);
		labelsStore.setItems(cfg.labels);
		userStateStore.reconcileFromServer(cfg.userState);
		troikiStore.setValue(cfg.troiki);
		planStatsStore.setValue(cfg.planStats);
		inboxStatsStore.set(cfg.inboxStats.count, cfg.inboxStats.warnThresholdExceeded);
		pinnedTasksStore.setItems(cfg.pinnedTasks);
		harpoonStore.setSlots(cfg.harpoon?.slots ?? []);
		templatesStore.setItems(cfg.taskTemplates ?? []);
		return cfg;
	}

	clear(): void {
		this.value = null;
	}
}

export const configStore = new ConfigStore();
