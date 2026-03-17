import { patchState } from '$lib/api/client';
import type { Context } from '$lib/api/types';
import { labelFilterStore } from './label-filter.svelte';

export type View = 'all' | 'inbox' | 'weekly' | 'backlog' | 'today' | 'tomorrow' | 'completed';

function createContextsStore() {
	let contexts = $state<Context[]>([]);
	let activeContextId = $state<string | null>(null);
	let activeView = $state<View>('all');

	function init(items: Context[], contextId: string, view: View): void {
		contexts = items;
		activeContextId = contextId || null;
		activeView = view;
		// Validate saved context still exists
		if (activeContextId && !contexts.some((c) => c.id === activeContextId)) {
			activeContextId = null;
			patchState({ active_context_id: '' }).catch(console.error);
		}
	}

	function setContext(id: string | null): void {
		activeContextId = id;
		labelFilterStore.clear();
		patchState({ active_context_id: id ?? '' }).catch(console.error);
	}

	function setView(view: View): void {
		activeView = view;
		labelFilterStore.clear();
		patchState({ active_view: view }).catch(console.error);
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
		init,
		setContext,
		setView
	};
}

export const contextsStore = createContextsStore();
