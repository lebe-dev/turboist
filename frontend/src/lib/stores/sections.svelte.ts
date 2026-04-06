import { logger } from '$lib/stores/logger';
import { isStateReady, persistUI } from '$lib/state/index.svelte';

function createSectionsStore() {
	let collapsed = $state(new Set<string>());
	let pinned = $state(new Set<string>());

	function syncCollapsed(): void {
		if (!isStateReady()) return;
		persistUI({ collapsed_section_ids: [...collapsed] });
	}

	function syncPinned(): void {
		if (!isStateReady()) return;
		persistUI({ pinned_section_ids: [...pinned] });
	}

	function init(initialCollapsed: string[], initialPinned: string[]): void {
		collapsed = new Set(initialCollapsed);
		pinned = new Set(initialPinned);
	}

	return {
		isCollapsed(id: string): boolean {
			return collapsed.has(id);
		},
		toggleCollapsed(id: string): void {
			if (collapsed.has(id)) {
				collapsed.delete(id);
			} else {
				collapsed.add(id);
			}
			collapsed = new Set(collapsed);
			syncCollapsed();
			logger.log('sections', `toggleCollapsed: ${id}`);
		},
		isPinned(id: string): boolean {
			return pinned.has(id);
		},
		togglePinned(id: string): void {
			if (pinned.has(id)) {
				pinned.delete(id);
			} else {
				pinned.add(id);
			}
			pinned = new Set(pinned);
			syncPinned();
			logger.log('sections', `togglePinned: ${id}`);
		},
		init
	};
}

export const sectionsStore = createSectionsStore();
