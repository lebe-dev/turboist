import { register, init, getLocaleFromNavigator, locale } from 'svelte-intl-precompile';
import { registerAll, availableLocales } from '$locales';

const STORAGE_KEY = 'turboist:locale';

export { availableLocales };

export function initLocale() {
	registerAll();

	const saved = localStorage.getItem(STORAGE_KEY);
	const browser = getLocaleFromNavigator()?.split('-')[0] ?? null;
	const initial = saved && availableLocales.includes(saved)
		? saved
		: browser && availableLocales.includes(browser)
			? browser
			: 'en';

	init({
		fallbackLocale: 'en',
		initialLocale: initial
	});

	// Persist locale changes to localStorage
	locale.subscribe((value: string) => {
		if (value) {
			localStorage.setItem(STORAGE_KEY, value);
		}
	});
}
