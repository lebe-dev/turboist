// Persistence for the project view's collapsed-subtask state, keyed per project
// so a folded set of parent tasks survives navigating away and back.
// Not to be confused with SUBTASK_COLLAPSE_KEY in lib/context/subtaskCollapse.ts,
// which is the Svelte context key — this is the localStorage namespace.
const STORAGE_PREFIX = 'turboist:subtaskCollapse:';

export function readCollapsedIds(projectId: number): number[] {
	if (typeof localStorage === 'undefined') return [];
	try {
		const raw = localStorage.getItem(STORAGE_PREFIX + projectId);
		if (!raw) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.filter((v): v is number => typeof v === 'number');
	} catch {
		return [];
	}
}

export function persistCollapsedIds(projectId: number, ids: Iterable<number>): void {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(STORAGE_PREFIX + projectId, JSON.stringify([...ids]));
	} catch {
		// ignore (quota / disabled storage)
	}
}
