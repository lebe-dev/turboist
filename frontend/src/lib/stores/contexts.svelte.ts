import { logger } from '$lib/stores/logger';
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
			logger.log('contexts', 'invalid saved context, resetting');
			patchState({ active_context_id: '' }).catch((e) => logger.error('contexts', String(e)));
		}
	}

	function setContext(id: string | null): void {
		logger.log('contexts', `setContext: ${id}`);
		activeContextId = id;
		labelFilterStore.clear();
		patchState({ active_context_id: id ?? '' }).catch((err) =>
			logger.error('contexts', `setContext save failed: ${err}`)
		);
	}

	function setView(view: View): void {
		logger.log('contexts', `setView: ${view}`);
		activeView = view;
		labelFilterStore.clear();
		patchState({ active_view: view }).catch((err) =>
			logger.error('contexts', `setView save failed: ${err}`)
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
