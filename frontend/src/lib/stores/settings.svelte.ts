import { settings as settingsApi } from '../api/endpoints/settings';
import { getApiClient } from '../api/client';
import type { BannerDayPart, UserSettings } from '../api/types';
import { setLocale, type SupportedLocale } from '../i18n';

/** Mirrors model.DefaultMaxPinned / MinMaxPinned / MaxMaxPinned on the server. */
export const DEFAULT_MAX_PINNED = 10;
export const MIN_MAX_PINNED = 1;
export const MAX_MAX_PINNED = 50;

const EMPTY: UserSettings = {
	weeklyUnplannedExcludedLabelIds: [],
	bugLabelIds: [],
	locale: '',
	publicView: false,
	bannerText: '',
	bannerPublished: false,
	bannerDayPart: '',
	calendarEnabled: false,
	calendarHidePastEvents: true,
	troikiEnabled: false,
	maxPinnedTasks: DEFAULT_MAX_PINNED,
	maxPinnedProjects: DEFAULT_MAX_PINNED
};

class SettingsStore {
	value = $state<UserSettings>({ ...EMPTY });

	setValue(v: UserSettings): void {
		this.value = v;
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

	get bannerDayPart(): BannerDayPart {
		return this.value.bannerDayPart ?? '';
	}

	get calendarEnabled(): boolean {
		return this.value.calendarEnabled ?? false;
	}

	get calendarHidePastEvents(): boolean {
		return this.value.calendarHidePastEvents ?? true;
	}

	get troikiEnabled(): boolean {
		return this.value.troikiEnabled ?? false;
	}

	get maxPinnedTasks(): number {
		return this.value.maxPinnedTasks ?? DEFAULT_MAX_PINNED;
	}

	get maxPinnedProjects(): number {
		return this.value.maxPinnedProjects ?? DEFAULT_MAX_PINNED;
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

	async setBannerDayPart(part: BannerDayPart): Promise<void> {
		this.value = { ...this.value, bannerDayPart: part };
		await settingsApi.patch(getApiClient(), { bannerDayPart: part });
	}

	async setCalendarEnabled(v: boolean): Promise<void> {
		this.value = { ...this.value, calendarEnabled: v };
		await settingsApi.patch(getApiClient(), { calendarEnabled: v });
	}

	async setCalendarHidePastEvents(v: boolean): Promise<void> {
		this.value = { ...this.value, calendarHidePastEvents: v };
		await settingsApi.patch(getApiClient(), { calendarHidePastEvents: v });
	}

	async setTroikiEnabled(v: boolean): Promise<void> {
		this.value = { ...this.value, troikiEnabled: v };
		await settingsApi.patch(getApiClient(), { troikiEnabled: v });
	}

	// Unlike the toggles above these are not written optimistically: the server
	// bounds them to [MIN_MAX_PINNED, MAX_MAX_PINNED], so a rejected PATCH must
	// not leave an unsaved number in the store. The response is the truth.
	async setMaxPinnedTasks(v: number): Promise<void> {
		this.value = await settingsApi.patch(getApiClient(), { maxPinnedTasks: v });
	}

	async setMaxPinnedProjects(v: number): Promise<void> {
		this.value = await settingsApi.patch(getApiClient(), { maxPinnedProjects: v });
	}

	clear(): void {
		this.value = { ...EMPTY };
	}
}

export const settingsStore = new SettingsStore();
