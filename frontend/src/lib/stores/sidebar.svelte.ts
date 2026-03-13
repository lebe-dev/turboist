const STORAGE_KEY = 'turboist:sidebar-collapsed';

function load(): boolean {
	try {
		return localStorage.getItem(STORAGE_KEY) === 'true';
	} catch {
		return false;
	}
}

function createSidebarStore() {
	let collapsed = $state(load());

	return {
		get collapsed(): boolean {
			return collapsed;
		},
		toggle(): void {
			collapsed = !collapsed;
			localStorage.setItem(STORAGE_KEY, String(collapsed));
		}
	};
}

export const sidebarStore = createSidebarStore();
