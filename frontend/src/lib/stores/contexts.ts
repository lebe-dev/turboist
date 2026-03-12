import { getContexts } from '$lib/api/client';
import type { Context } from '$lib/api/types';

export type View = 'all' | 'weekly' | 'next-week';

function createContextsStore() {
	let contexts = $state<Context[]>([]);
	let activeContextId = $state<string | null>(null);
	let activeView = $state<View>('all');

	async function load(): Promise<void> {
		contexts = await getContexts();
	}

	function setContext(id: string | null): void {
		activeContextId = id;
	}

	function setView(view: View): void {
		activeView = view;
	}

	return {
		get contexts() {
			return contexts;
		},
		get activeContextId() {
			return activeContextId;
		},
		get activeView() {
			return activeView;
		},
		load,
		setContext,
		setView
	};
}

export const contextsStore = createContextsStore();
