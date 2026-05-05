import { settings as settingsApi } from '../api/endpoints/settings';
import { getApiClient } from '../api/client';
import type { UserSettings } from '../api/types';

class SettingsStore {
	value = $state<UserSettings>({ weeklyUnplannedExcludedLabelIds: [] });

	async load(): Promise<UserSettings> {
		const v = await settingsApi.get(getApiClient());
		this.value = v;
		return v;
	}

	get weeklyUnplannedExcludedLabelIds(): number[] {
		return this.value.weeklyUnplannedExcludedLabelIds ?? [];
	}

	async setWeeklyUnplannedExcludedLabelIds(ids: number[]): Promise<void> {
		this.value = { ...this.value, weeklyUnplannedExcludedLabelIds: ids };
		await settingsApi.patch(getApiClient(), { weeklyUnplannedExcludedLabelIds: ids });
	}

	clear(): void {
		this.value = { weeklyUnplannedExcludedLabelIds: [] };
	}
}

export const settingsStore = new SettingsStore();
