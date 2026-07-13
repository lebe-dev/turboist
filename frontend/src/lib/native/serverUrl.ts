import { Preferences } from '@capacitor/preferences';
import { isNativePlatform } from './platform';

// The remote API base URL entered by the user on first native launch. It is
// configuration, not a secret, so plain Preferences (UserDefaults / Shared-
// Preferences) is fine. On web this module is inert: the base URL is always ''
// (same-origin) and nothing is persisted.

const KEY = 'serverUrl';
let cached = '';

/** Strip a trailing slash so it composes with API paths that start with '/'. */
export function normalizeServerUrl(raw: string): string {
	return raw.trim().replace(/\/+$/, '');
}

/**
 * Load the persisted server URL into the module cache. Returns '' on web (and
 * when nothing is stored yet). Call once during app bootstrap, before any
 * ApiClient is constructed.
 */
export async function loadServerUrl(): Promise<string> {
	if (!isNativePlatform()) {
		cached = '';
		return '';
	}
	const { value } = await Preferences.get({ key: KEY });
	cached = value ?? '';
	return cached;
}

/** Synchronous accessor — valid only after loadServerUrl() has resolved. */
export function getServerUrl(): string {
	return cached;
}

export async function saveServerUrl(url: string): Promise<void> {
	cached = normalizeServerUrl(url);
	await Preferences.set({ key: KEY, value: cached });
}

export async function clearServerUrl(): Promise<void> {
	cached = '';
	await Preferences.remove({ key: KEY });
}
