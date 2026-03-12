const STORAGE_KEY = 'turboist:collapsed';

function load(): Set<string> {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) return new Set(JSON.parse(raw));
	} catch {
		// ignore
	}
	return new Set();
}

function save(ids: Set<string>): void {
	localStorage.setItem(STORAGE_KEY, JSON.stringify([...ids]));
}

function createCollapsedStore() {
	let ids = $state(load());

	return {
		get hasAny(): boolean {
			return ids.size > 0;
		},
		isCollapsed(id: string): boolean {
			return ids.has(id);
		},
		toggle(id: string): void {
			if (ids.has(id)) {
				ids.delete(id);
			} else {
				ids.add(id);
			}
			ids = new Set(ids);
			save(ids);
		},
		collapseAll(taskIds: string[]): void {
			ids = new Set(taskIds);
			save(ids);
		},
		expandAll(): void {
			ids = new Set();
			save(ids);
		}
	};
}

export const collapsedStore = createCollapsedStore();
