import { logger } from '$lib/stores/logger';
import { patchState } from '$lib/api/client';

function createSidebarStore() {
	let collapsed = $state(false);

	function init(initialCollapsed: boolean): void {
		collapsed = initialCollapsed;
	}

	return {
		get collapsed(): boolean {
			return collapsed;
		},
		toggle(): void {
			collapsed = !collapsed;
			logger.log('sidebar', `toggle: ${collapsed}`);
			patchState({ sidebar_collapsed: collapsed }).catch((err) =>
				logger.error('sidebar', `toggle save failed: ${err}`)
			);
		},
		init
	};
}

export const sidebarStore = createSidebarStore();
