import { appSettings as appSettingsApi } from '../api/endpoints/app-settings';
import { getApiClient } from '../api/client';
import type { AppSettings, AutoLabelRule } from '../api/types';

function createAppSettingsStore() {
	let value = $state<AppSettings>({ autoLabels: [] });

	return {
		get value(): AppSettings {
			return value;
		},
		get autoLabels(): AutoLabelRule[] {
			return value.autoLabels;
		},
		async load(): Promise<AppSettings> {
			const v = await appSettingsApi.get(getApiClient());
			value = v;
			return v;
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
		clear(): void {
			value = { autoLabels: [] };
		}
	};
}

export const appSettingsStore = createAppSettingsStore();
