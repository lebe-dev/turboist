import { settings as settingsApi } from '../api/endpoints/settings';
import { getApiClient } from '../api/client';
import type { UserSettings } from '../api/types';
import { setLocale, type SupportedLocale } from '../i18n';

const EMPTY: UserSettings = { weeklyUnplannedExcludedLabelIds: [], locale: '', publicView: false };

class SettingsStore {
	value = $state<UserSettings>({ ...EMPTY });

	async load(): Promise<UserSettings> {
		const v = await settingsApi.get(getApiClient());
		this.value = v;
		return v;
	}

	get weeklyUnplannedExcludedLabelIds(): number[] {
		return this.value.weeklyUnplannedExcludedLabelIds ?? [];
	}

	get locale(): string {
		return this.value.locale ?? '';
	}

	get publicView(): boolean {
		return this.value.publicView ?? false;
	}

	async setWeeklyUnplannedExcludedLabelIds(ids: number[]): Promise<void> {
		this.value = { ...this.value, weeklyUnplannedExcludedLabelIds: ids };
		await settingsApi.patch(getApiClient(), { weeklyUnplannedExcludedLabelIds: ids });
	}

	async setLocale(loc: SupportedLocale): Promise<void> {
		const updated = await settingsApi.patch(getApiClient(), { locale: loc });
		this.value = updated;
		setLocale(loc);
	}

	async setPublicView(v: boolean): Promise<void> {
		this.value = { ...this.value, publicView: v };
		await settingsApi.patch(getApiClient(), { publicView: v });
	}

	clear(): void {
		this.value = { ...EMPTY };
	}
}

export const settingsStore = new SettingsStore();
