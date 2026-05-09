import { init, getLocaleFromNavigator, locale, t } from 'svelte-intl-precompile';
import { registerAll, availableLocales } from '$locales';

export { availableLocales, locale, t };

export const SUPPORTED_LOCALES = ['en', 'ru'] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export function isSupportedLocale(value: string | null | undefined): value is SupportedLocale {
	return !!value && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

let initialized = false;

export function initI18n(preferred: string | null): void {
	if (initialized) return;
	registerAll();
	const browser = getLocaleFromNavigator()?.split('-')[0] ?? null;
	const resolved =
		(isSupportedLocale(preferred) && preferred) ||
		(isSupportedLocale(browser) && browser) ||
		'en';
	init({ fallbackLocale: 'en', initialLocale: resolved });
	initialized = true;
}

export function setLocale(value: SupportedLocale): void {
	locale.set(value);
}

export function localeLabel(value: SupportedLocale): string {
	switch (value) {
		case 'en':
			return 'English';
		case 'ru':
			return 'Русский';
	}
}
