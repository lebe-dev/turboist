// Pure ranking for the quick switcher (⌘/Ctrl+P): what to show for a query, and
// in what order. The rules mirror the server's search ranking so the online
// answer, the offline answer built from the read cache and the instant answer
// from the remembered recents all agree:
//
// 1. Recently opened tasks and projects that match come first, most recent
//    first — the switcher is mostly used to get back to something.
// 2. Everything else follows by where the words hit: title, then a label, then
//    the description. Within one rank projects lead (there are few of them and
//    they are places), and the incoming order is kept, so the server's own
//    tie-breaks (open before finished, pinned, priority) survive.

import type { Project, Task } from '$lib/api/types';

export type SwitcherKind = 'task' | 'project';

/** A remembered recently-opened task or project (see `recentItems` store). */
export interface RecentEntry {
	kind: SwitcherKind;
	id: number;
	title: string;
	labels: string[];
}

export interface SwitcherResult {
	kind: SwitcherKind;
	id: number;
	title: string;
	recent: boolean;
	/** The full record when the search returned it; absent for a remembered snapshot. */
	task?: Task;
	project?: Project;
}

/** Match rank: 0 = title, 1 = a label, 2 = the description. */
export type MatchRank = 0 | 1 | 2;

export interface Matchable {
	title: string;
	labels: readonly string[];
	description?: string;
}

/** Unicode-aware case folding, the client twin of the server's `casefold`. */
export function foldCase(s: string): string {
	return s.toLowerCase();
}

/** Where `query` hits `item`, or null when it does not (or the query is blank). */
export function matchRank(query: string, item: Matchable): MatchRank | null {
	const q = foldCase(query.trim());
	if (!q) return null;
	if (foldCase(item.title).includes(q)) return 0;
	if (item.labels.some((l) => foldCase(l).includes(q))) return 1;
	if (item.description && foldCase(item.description).includes(q)) return 2;
	return null;
}

function key(kind: SwitcherKind, id: number): string {
	return `${kind}:${id}`;
}

function taskMatchable(t: Task): Matchable {
	return { title: t.title, labels: t.labels.map((l) => l.name), description: t.description };
}

function projectMatchable(p: Project): Matchable {
	return { title: p.title, labels: p.labels.map((l) => l.name), description: p.description };
}

/**
 * The rows the switcher shows for `query`.
 *
 * `recents` is most-recent-first. `tasks`/`projects` are the search answer in
 * the order it came back (may be empty — offline with nothing cached, or the
 * request not answered yet). A blank query shows the recents alone.
 */
export function switcherResults(
	query: string,
	recents: readonly RecentEntry[],
	tasks: readonly Task[],
	projects: readonly Project[]
): SwitcherResult[] {
	const q = query.trim();
	if (!q) {
		return recents.map((r) => ({ kind: r.kind, id: r.id, title: r.title, recent: true }));
	}

	const fresh = new Map<string, SwitcherResult & { rank: MatchRank; order: number }>();
	let order = 0;
	for (const p of projects) {
		const rank = matchRank(q, projectMatchable(p));
		// The server may match on something the client cannot see (e.g. a label it
		// does not carry); trust it and rank such a hit last.
		fresh.set(key('project', p.id), {
			kind: 'project',
			id: p.id,
			title: p.title,
			recent: false,
			project: p,
			rank: rank ?? 2,
			order: order++
		});
	}
	for (const t of tasks) {
		const rank = matchRank(q, taskMatchable(t));
		fresh.set(key('task', t.id), {
			kind: 'task',
			id: t.id,
			title: t.title,
			recent: false,
			task: t,
			rank: rank ?? 2,
			order: order++
		});
	}

	const head: SwitcherResult[] = [];
	for (const r of recents) {
		const k = key(r.kind, r.id);
		const hit = fresh.get(k);
		if (hit) {
			fresh.delete(k);
			const { rank: _rank, order: _order, ...row } = hit;
			head.push({ ...row, recent: true });
			continue;
		}
		if (matchRank(q, r) === null) continue;
		head.push({ kind: r.kind, id: r.id, title: r.title, recent: true });
	}

	const tail = [...fresh.values()].sort((a, b) => {
		if (a.rank !== b.rank) return a.rank - b.rank;
		if (a.kind !== b.kind) return a.kind === 'project' ? -1 : 1;
		return a.order - b.order;
	});
	return [...head, ...tail.map(({ rank: _rank, order: _order, ...row }) => row)];
}

/** How many "add for today" rows the switcher offers. */
export const TODAY_TARGETS = 2;

/**
 * Where "add for today" can file a task: the most recently opened projects that
 * still exist and are open, most recent first. Empty means no "for today" row at
 * all — the Inbox holds undated capture only, so it is never a "today" target.
 */
export function todayTargetProjects<P extends { id: number; status: string }>(
	recentProjectIds: readonly number[],
	projects: readonly P[],
	limit: number = TODAY_TARGETS
): P[] {
	const byId = new Map(projects.map((p) => [p.id, p]));
	const out: P[] = [];
	for (const id of recentProjectIds) {
		if (out.length >= limit) break;
		const p = byId.get(id);
		if (p && p.status === 'open') out.push(p);
	}
	return out;
}
