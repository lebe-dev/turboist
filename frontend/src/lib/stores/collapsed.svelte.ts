import { logger } from '$lib/stores/logger';
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
			logger.log('collapsed', `toggle: ${id} total: ${ids.size}`);
			patchState({ collapsed_ids: [...ids] }).catch((err) =>
				logger.error('collapsed', `toggle save failed: ${err}`)
			);
		},
		collapseAll(taskIds: string[]): void {
			ids = new Set(taskIds);
			logger.log('collapsed', `collapseAll: ${taskIds.length}`);
			patchState({ collapsed_ids: [...ids] }).catch((err) =>
				logger.error('collapsed', `collapseAll save failed: ${err}`)
			);
		},
		expandAll(): void {
			ids = new Set();
			logger.log('collapsed', 'expandAll');
			patchState({ collapsed_ids: [] }).catch((err) =>
				logger.error('collapsed', `expandAll save failed: ${err}`)
			);
		},
		init
	};
}

export const collapsedStore = createCollapsedStore();
