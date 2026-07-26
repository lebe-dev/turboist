import { appSettings as appSettingsApi } from '../api/endpoints/app-settings';
import { getApiClient } from '../api/client';
import type { AppSettings, AutoLabelRule, ProjectSuggestionRule } from '../api/types';

function emptyAppSettings(): AppSettings {
	return { autoLabels: [], projectSuggestions: [] };
}

function createAppSettingsStore() {
	let value = $state<AppSettings>(emptyAppSettings());

	return {
		get value(): AppSettings {
			return value;
		},
		get autoLabels(): AutoLabelRule[] {
			return value.autoLabels;
		},
		get projectSuggestions(): ProjectSuggestionRule[] {
			return value.projectSuggestions ?? [];
		},
		async load(): Promise<AppSettings> {
			const v = await appSettingsApi.get(getApiClient());
			value = v;
			return v;
		},
		setValue(v: AppSettings): void {
			value = v;
		},
		async setAutoLabels(rules: AutoLabelRule[]): Promise<void> {
			const prev = value;
			value = { ...value, autoLabels: rules };
			try {
				const updated = await appSettingsApi.setAutoLabels(getApiClient(), rules);
				value = updated;
			} catch (err) {
				value = prev;
				throw err;
			}
		},
		async setProjectSuggestions(rules: ProjectSuggestionRule[]): Promise<void> {
			const prev = value;
			value = { ...value, projectSuggestions: rules };
			try {
				const updated = await appSettingsApi.setProjectSuggestions(getApiClient(), rules);
				value = updated;
			} catch (err) {
				value = prev;
				throw err;
			}
		},
		clear(): void {
			value = emptyAppSettings();
		}
	};
}

export const appSettingsStore = createAppSettingsStore();
