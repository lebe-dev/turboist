// Most-recently-applied labels, kept client-side.
//
// Label pickers surface the last few labels the user actually applied to a
// task at the top of their list — applying a label to a task is the only
// signal we record (removing one is not). The order is local to this device
// (localStorage): it is a habit, not user data worth a server round-trip.
//
// More ids are kept than any picker shows, because pickers filter their
// candidates first (search query, visibility) — a deeper history still
// yields a full row of recents after filtering.

const STORAGE_KEY = 'turboist:recentLabels';

/** How many ids are remembered. */
export const RECENT_LABELS_MEMORY = 12;

/** How many recents a picker shows by default. */
export const RECENT_LABELS_SHOWN = 5;

function loadInitial(): number[] {
	if (typeof localStorage === 'undefined') return [];
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed
			.filter((v): v is number => typeof v === 'number' && Number.isFinite(v))
			.slice(0, RECENT_LABELS_MEMORY);
	} catch {
		return [];
	}
}

class RecentLabelsStore {
	/** Label ids, most recently applied first. */
	ids = $state<number[]>(loadInitial());

	/** Record that the user applied a label; moves it to the front. */
	visit(id: number): void {
		if (!Number.isFinite(id)) return;
		if (this.ids[0] === id) return;
		this.ids = [id, ...this.ids.filter((x) => x !== id)].slice(0, RECENT_LABELS_MEMORY);
		this.persist();
	}

	/**
	 * The recent slice of `candidates`, most recently applied first. Callers
	 * pass their already-filtered list, so a label the picker hides (no search
	 * match, deleted) never shows up here either.
	 *
	 * Returns nothing when the list is already short enough to fit in the
	 * recent row — a "recent" group that repeats the whole picker is pure
	 * noise.
	 */
	pick<T extends { id: number }>(
		candidates: readonly T[],
		limit: number = RECENT_LABELS_SHOWN
	): T[] {
		if (limit <= 0 || candidates.length <= limit) return [];
		const byId = new Map(candidates.map((c) => [c.id, c]));
		const out: T[] = [];
		for (const id of this.ids) {
			const hit = byId.get(id);
			if (!hit) continue;
			out.push(hit);
			if (out.length === limit) break;
		}
		return out;
	}

	clear(): void {
		this.ids = [];
		this.persist();
	}

	private persist(): void {
		if (typeof localStorage === 'undefined') return;
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(this.ids));
		} catch {
			// ignore (quota / disabled storage)
		}
	}
}

export const recentLabelsStore = new RecentLabelsStore();
