import { patchState } from '$lib/api/client';

function createCollapsedStore() {
	let ids = $state(new Set<string>());

	function init(initialIds: string[]): void {
		ids = new Set(initialIds);
	}

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
			patchState({ collapsed_ids: [...ids] }).catch(console.error);
		},
		collapseAll(taskIds: string[]): void {
			ids = new Set(taskIds);
			patchState({ collapsed_ids: [...ids] }).catch(console.error);
		},
		expandAll(): void {
			ids = new Set();
			patchState({ collapsed_ids: [] }).catch(console.error);
		},
		init
	};
}

export const collapsedStore = createCollapsedStore();
