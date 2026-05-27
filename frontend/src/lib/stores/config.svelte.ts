import { config as configApi } from '../api/endpoints/config';
import { getApiClient } from '../api/client';
import type { ConfigResponse } from '../api/types';
import { contextsStore } from './contexts.svelte';
import { projectsStore } from './projects.svelte';
import { labelsStore } from './labels.svelte';
import { settingsStore } from './settings.svelte';
import { appSettingsStore } from './appSettings.svelte';
import { userStateStore } from './userState.svelte';
import { troikiStore } from './troiki.svelte';

class ConfigStore {
	value = $state<ConfigResponse | null>(null);

	// load fetches the workspace bootstrap in one round-trip and fans the
	// embedded sections out to the per-domain stores. Replaces the previous
	// eight parallel GETs (contexts, projects, labels, settings, app-settings,
	// state, troiki, config) issued from `+layout.svelte` on every page load.
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
		return cfg;
	}

	clear(): void {
		this.value = null;
	}
}

export const configStore = new ConfigStore();
