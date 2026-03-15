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
			patchState({ sidebar_collapsed: collapsed }).catch(console.error);
		},
		init
	};
}

export const sidebarStore = createSidebarStore();
