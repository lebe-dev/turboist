// Most-recently-opened tasks and projects, kept client-side for the quick
// switcher (⌘/Ctrl+P).
//
// One list for both kinds, so "most recent first" means the same thing across
// them. Each entry carries a snapshot of the title and label names: the switcher
// shows the recents the instant it opens — before any request, and with no
// network at all — and filters them locally while the search is in flight.
// The snapshot is refreshed every time the page is opened again, and a search
// answer that names the same row replaces it on screen.
//
// Device-local (localStorage) for the same reason the project pickers' MRU is: a
// navigation habit, not workspace data worth a server round-trip.

import type { RecentEntry, SwitcherKind } from '$lib/utils/quickSwitcher';

const STORAGE_KEY = 'turboist:recentItems';

/** How many entries are remembered. */
export const RECENT_ITEMS_MEMORY = 30;

/** How many the switcher shows for an empty query. */
export const RECENT_ITEMS_SHOWN = 8;

function isEntry(v: unknown): v is RecentEntry {
	if (!v || typeof v !== 'object') return false;
	const e = v as Record<string, unknown>;
	return (
		(e.kind === 'task' || e.kind === 'project') &&
		typeof e.id === 'number' &&
		Number.isFinite(e.id) &&
		typeof e.title === 'string' &&
		Array.isArray(e.labels) &&
		e.labels.every((l) => typeof l === 'string')
	);
}

function loadInitial(): RecentEntry[] {
	if (typeof localStorage === 'undefined') return [];
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.filter(isEntry).slice(0, RECENT_ITEMS_MEMORY);
	} catch {
		return [];
	}
}

class RecentItemsStore {
	/** Most recent first. */
	entries = $state<RecentEntry[]>(loadInitial());

	/** Record that the user opened a task or project; moves it to the front. */
	visit(entry: RecentEntry): void {
		if (!Number.isFinite(entry.id)) return;
		const snapshot: RecentEntry = {
			kind: entry.kind,
			id: entry.id,
			title: entry.title,
			labels: [...entry.labels]
		};
		const first = this.entries[0];
		if (
			first &&
			first.kind === snapshot.kind &&
			first.id === snapshot.id &&
			first.title === snapshot.title &&
			first.labels.join('\n') === snapshot.labels.join('\n')
		) {
			return;
		}
		this.entries = [
			snapshot,
			...this.entries.filter((e) => !(e.kind === snapshot.kind && e.id === snapshot.id))
		].slice(0, RECENT_ITEMS_MEMORY);
		this.persist();
	}

	/** Drop an entry whose row is gone (deleted, or not found when opened). */
	forget(kind: SwitcherKind, id: number): void {
		const next = this.entries.filter((e) => !(e.kind === kind && e.id === id));
		if (next.length === this.entries.length) return;
		this.entries = next;
		this.persist();
	}

	clear(): void {
		this.entries = [];
		this.persist();
	}

	private persist(): void {
		if (typeof localStorage === 'undefined') return;
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(this.entries));
		} catch {
			// ignore (quota / disabled storage)
		}
	}
}

export const recentItemsStore = new RecentItemsStore();
