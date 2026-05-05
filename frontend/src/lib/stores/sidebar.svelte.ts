const STORAGE_KEY = 'turboist:sidebar-collapsed';

function loadInitial(): boolean {
	if (typeof localStorage === 'undefined') return false;
	return localStorage.getItem(STORAGE_KEY) === '1';
}

class SidebarStore {
	collapsed = $state<boolean>(loadInitial());

	toggle(): void {
		this.collapsed = !this.collapsed;
		this.persist();
	}

	set(value: boolean): void {
		this.collapsed = value;
		this.persist();
	}

	private persist(): void {
		if (typeof localStorage === 'undefined') return;
		localStorage.setItem(STORAGE_KEY, this.collapsed ? '1' : '0');
	}
}

export const sidebarStore = new SidebarStore();
