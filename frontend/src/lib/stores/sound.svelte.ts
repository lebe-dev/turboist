import { playStatusTone } from '$lib/utils/sound';

const STORAGE_KEY = 'turboist:sound-enabled';

/**
 * Sound is a per-device preference (a phone in a meeting and a desktop at home
 * want different answers), so it lives in localStorage next to the sidebar flag
 * rather than in the server-side `users.settings` blob. Default on.
 */
function loadInitial(): boolean {
	if (typeof localStorage === 'undefined') return true;
	return localStorage.getItem(STORAGE_KEY) !== '0';
}

function createSoundStore() {
	let enabled = $state<boolean>(loadInitial());

	function persist(): void {
		if (typeof localStorage === 'undefined') return;
		localStorage.setItem(STORAGE_KEY, enabled ? '1' : '0');
	}

	return {
		get enabled(): boolean {
			return enabled;
		},
		setEnabled(value: boolean): void {
			enabled = value;
			persist();
		},
		/** Called from every task-status toggle; a no-op when the user muted it. */
		playTaskStatus(completed: boolean): void {
			if (!enabled) return;
			playStatusTone(completed);
		}
	};
}

export const soundStore = createSoundStore();
