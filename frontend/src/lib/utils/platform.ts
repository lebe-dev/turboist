import { browser } from '$app/environment';

export function isMacOS(): boolean {
	if (!browser) return false;
	return /Mac|iPhone|iPad|iPod/.test(navigator.platform);
}

export function modShortcut(key: string): string {
	return isMacOS() ? `⌘${key.toUpperCase()}` : `Ctrl+${key.toUpperCase()}`;
}
