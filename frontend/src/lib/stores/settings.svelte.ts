import { settings as settingsApi } from '../api/endpoints/settings';
import { getApiClient } from '../api/client';
import type { UserSettings } from '../api/types';
import { setLocale, type SupportedLocale } from '../i18n';

const EMPTY: UserSettings = {
	weeklyUnplannedExcludedLabelIds: [],
	bugLabelIds: [],
	locale: '',
	publicView: false,
	bannerText: '',
	bannerPublished: false
};

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

	get bugLabelIds(): number[] {
		return this.value.bugLabelIds ?? [];
	}

	get locale(): string {
		return this.value.locale ?? '';
	}

	get publicView(): boolean {
		return this.value.publicView ?? false;
	}

	get bannerText(): string {
		return this.value.bannerText ?? '';
	}

	get bannerPublished(): boolean {
		return this.value.bannerPublished ?? false;
	}

	async setWeeklyUnplannedExcludedLabelIds(ids: number[]): Promise<void> {
		this.value = { ...this.value, weeklyUnplannedExcludedLabelIds: ids };
		await settingsApi.patch(getApiClient(), { weeklyUnplannedExcludedLabelIds: ids });
	}

	async setBugLabelIds(ids: number[]): Promise<void> {
		this.value = { ...this.value, bugLabelIds: ids };
		await settingsApi.patch(getApiClient(), { bugLabelIds: ids });
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

	async setBannerText(text: string): Promise<void> {
		this.value = { ...this.value, bannerText: text };
		await settingsApi.patch(getApiClient(), { bannerText: text });
	}

	async setBannerPublished(v: boolean): Promise<void> {
		this.value = { ...this.value, bannerPublished: v };
		await settingsApi.patch(getApiClient(), { bannerPublished: v });
	}

	clear(): void {
		this.value = { ...EMPTY };
	}
}

export const settingsStore = new SettingsStore();
