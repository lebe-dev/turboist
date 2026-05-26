import { waitLocale } from 'svelte-intl-precompile';
import { initI18n } from '$lib/i18n';

export const ssr = false;
export const prerender = false;
export const trailingSlash = 'never';

export const load = async () => {
	initI18n(null);
	await waitLocale();
};
