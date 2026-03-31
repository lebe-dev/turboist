import { register, init, getLocaleFromNavigator, locale } from 'svelte-intl-precompile';
import { registerAll, availableLocales } from '$locales';

export { availableLocales };

export function initLocale() {
	registerAll();

	const browser = getLocaleFromNavigator()?.split('-')[0] ?? null;
	const initial = browser && availableLocales.includes(browser) ? browser : 'en';

	init({
		fallbackLocale: 'en',
		initialLocale: initial
	});
}

export function applyLocaleFromConfig(loc: string): void {
	if (loc && availableLocales.includes(loc)) {
		locale.set(loc);
	}
}
