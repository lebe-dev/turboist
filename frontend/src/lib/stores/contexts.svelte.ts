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
			console.log('[contexts] invalid saved context, resetting');
			patchState({ active_context_id: '' }).catch(console.error);
		}
	}

	function setContext(id: string | null): void {
		console.log('[contexts] setContext:', id);
		activeContextId = id;
		labelFilterStore.clear();
		patchState({ active_context_id: id ?? '' }).catch((err) =>
			console.error('[contexts] setContext save failed:', err)
		);
	}

	function setView(view: View): void {
		console.log('[contexts] setView:', view);
		activeView = view;
		labelFilterStore.clear();
		patchState({ active_view: view }).catch((err) =>
			console.error('[contexts] setView save failed:', err)
		);
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
