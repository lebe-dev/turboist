import { appSettings as appSettingsApi } from '../api/endpoints/app-settings';
import { getApiClient } from '../api/client';
import type {
	AppSettings,
	AutoLabelRule,
	InboxProcessingSettings,
	ProjectSuggestionRule
} from '../api/types';

function emptyAppSettings(): AppSettings {
	return { autoLabels: [], projectSuggestions: [], inboxProcessing: { prompt: '', paused: false } };
}

// Settings payloads from before a field existed decode without it.
function inboxOf(v: AppSettings): InboxProcessingSettings {
	const stored: Partial<InboxProcessingSettings> = v.inboxProcessing ?? {};
	return { prompt: stored.prompt ?? '', paused: stored.paused ?? false };
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
		get inboxProcessing(): InboxProcessingSettings {
			return inboxOf(value);
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
		async setInboxProcessingPrompt(prompt: string): Promise<void> {
			const prev = value;
			value = { ...value, inboxProcessing: { ...inboxOf(value), prompt } };
			try {
				const updated = await appSettingsApi.setInboxProcessingPrompt(getApiClient(), prompt);
				value = updated;
			} catch (err) {
				value = prev;
				throw err;
			}
		},
		async setInboxProcessingPaused(paused: boolean): Promise<void> {
			const prev = value;
			value = { ...value, inboxProcessing: { ...inboxOf(value), paused } };
			try {
				const updated = await appSettingsApi.setInboxProcessingPaused(getApiClient(), paused);
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
