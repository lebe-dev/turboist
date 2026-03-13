import { getContexts } from '$lib/api/client';
import type { Context } from '$lib/api/types';

export type View = 'all' | 'inbox' | 'weekly' | 'next-week' | 'today' | 'tomorrow' | 'completed';

const CONTEXT_KEY = 'turboist:context';
const VIEW_KEY = 'turboist:view';

const VALID_VIEWS: View[] = ['all', 'inbox', 'today', 'tomorrow', 'weekly', 'next-week', 'completed'];

function loadContext(): string | null {
	try {
		return localStorage.getItem(CONTEXT_KEY) || null;
	} catch {
		return null;
	}
}

function loadView(): View {
	try {
		const v = localStorage.getItem(VIEW_KEY) as View | null;
		if (v && VALID_VIEWS.includes(v)) return v;
	} catch {
		// ignore
	}
	return 'all';
}

function createContextsStore() {
	let contexts = $state<Context[]>([]);
	let activeContextId = $state<string | null>(loadContext());
	let activeView = $state<View>(loadView());

	async function load(): Promise<void> {
		contexts = await getContexts();
		// Validate saved context still exists
		if (activeContextId && !contexts.some((c) => c.id === activeContextId)) {
			activeContextId = null;
			localStorage.removeItem(CONTEXT_KEY);
		}
	}

	function setContext(id: string | null): void {
		activeContextId = id;
		if (id) {
			localStorage.setItem(CONTEXT_KEY, id);
		} else {
			localStorage.removeItem(CONTEXT_KEY);
		}
	}

	function setView(view: View): void {
		activeView = view;
		localStorage.setItem(VIEW_KEY, view);
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
