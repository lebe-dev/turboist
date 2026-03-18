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
			console.log('[collapsed] toggle:', id, 'total:', ids.size);
			patchState({ collapsed_ids: [...ids] }).catch((err) =>
				console.error('[collapsed] toggle save failed:', err)
			);
		},
		collapseAll(taskIds: string[]): void {
			ids = new Set(taskIds);
			console.log('[collapsed] collapseAll:', taskIds.length);
			patchState({ collapsed_ids: [...ids] }).catch((err) =>
				console.error('[collapsed] collapseAll save failed:', err)
			);
		},
		expandAll(): void {
			ids = new Set();
			console.log('[collapsed] expandAll');
			patchState({ collapsed_ids: [] }).catch((err) =>
				console.error('[collapsed] expandAll save failed:', err)
			);
		},
		init
	};
}

export const collapsedStore = createCollapsedStore();
