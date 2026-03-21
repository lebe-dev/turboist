import { logger } from '$lib/stores/logger';
import { patchState } from '$lib/api/client';
import { isStateReady, persistUI } from '$lib/state/index.svelte';

function createSidebarStore() {
	let collapsed = $state(false);

	function syncToState(value: boolean): void {
		if (!isStateReady()) return;
		persistUI({ sidebar_collapsed: value });
	}

	function init(initialCollapsed: boolean): void {
		collapsed = initialCollapsed;
		syncToState(initialCollapsed);
	}

	return {
		get collapsed(): boolean {
			return collapsed;
		},
		toggle(): void {
			collapsed = !collapsed;
			syncToState(collapsed);
			logger.log('sidebar', `toggle: ${collapsed}`);
			patchState({ sidebar_collapsed: collapsed }).catch((err) =>
				logger.error('sidebar', `toggle save failed: ${err}`)
			);
		},
		init
	};
}

export const sidebarStore = createSidebarStore();
