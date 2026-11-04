// Offline answer for `GET /api/v1/search`.
//
// The read-through cache serves a search only for the exact query it has seen
// before, which is useless while typing. When the network is down and there is no
// exact hit, the search is answered from every task and project the cache holds,
// ranked by the same rules as the server (title, then a label, then the
// description; open before finished). The answer is necessarily partial — only
// what some screen has already loaded — but it is the workspace the user has been
// looking at, which is what they are usually trying to get back to.

import type { Project, SearchResponse, Task } from '$lib/api/types';
import { matchRank } from '$lib/utils/quickSwitcher';
import type { CachedResponse } from './db';
import { candidateLists, looksLikeTask } from './readCache';

export interface SearchFallbackQuery {
	q?: unknown;
	type?: unknown;
	limit?: unknown;
	offset?: unknown;
}

const DEFAULT_LIMIT = 50;

export function matchSearchPath(path: string): boolean {
	return path.split('?', 1)[0] === '/api/v1/search';
}

function looksLikeProject(value: unknown): value is Project {
	if (!value || typeof value !== 'object') return false;
	const o = value as Record<string, unknown>;
	return (
		typeof o.id === 'number' &&
		typeof o.title === 'string' &&
		'status' in o &&
		!looksLikeTask(value) &&
		('projectType' in o || 'contextId' in o || 'color' in o || 'labels' in o)
	);
}

/** Every project array a cached response holds. */
function projectLists(entry: CachedResponse): unknown[][] {
	const path = entry.path.split('?', 1)[0];
	const p = entry.payload as Record<string, unknown> | null;
	if (!p || typeof p !== 'object') return [];
	if (path === '/api/v1/config' && Array.isArray(p.projects)) return [p.projects];
	if (path === '/api/v1/projects' && Array.isArray(p.items)) return [p.items];
	if (path === '/api/v1/search') {
		const items = (p.projects as { items?: unknown } | undefined)?.items;
		return Array.isArray(items) ? [items] : [];
	}
	if (/^\/api\/v1\/projects\/-?\d+\/bundle$/.test(path) && p.project) return [[p.project]];
	return [];
}

function toInt(v: unknown, fallback: number): number {
	const n = typeof v === 'number' ? v : Number(v);
	return Number.isFinite(n) && n >= 0 ? Math.floor(n) : fallback;
}

function rankAndPage<T extends { id: number; title: string; description?: string; status: string; labels?: { name: string }[] }>(
	items: Iterable<T>,
	q: string,
	limit: number,
	offset: number
): { items: T[]; total: number; limit: number; offset: number } {
	const hits: { item: T; rank: number }[] = [];
	for (const item of items) {
		const rank = matchRank(q, {
			title: item.title,
			labels: (item.labels ?? []).map((l) => l.name),
			description: item.description ?? ''
		});
		if (rank !== null) hits.push({ item, rank });
	}
	hits.sort((a, b) => {
		if (a.rank !== b.rank) return a.rank - b.rank;
		const ao = a.item.status === 'open' ? 0 : 1;
		const bo = b.item.status === 'open' ? 0 : 1;
		if (ao !== bo) return ao - bo;
		return b.item.id - a.item.id;
	});
	return {
		items: hits.slice(offset, offset + limit).map((h) => h.item),
		total: hits.length,
		limit,
		offset
	};
}

/** Build a SearchResponse from every cached entry. */
export function searchFromCache(
	entries: readonly CachedResponse[],
	query: SearchFallbackQuery
): SearchResponse {
	const q = typeof query.q === 'string' ? query.q : '';
	const type = query.type === 'tasks' || query.type === 'projects' ? query.type : 'all';
	const limit = toInt(query.limit, DEFAULT_LIMIT) || DEFAULT_LIMIT;
	const offset = toInt(query.offset, 0);

	// Freshest copy wins when a row is cached under several endpoints.
	const ordered = [...entries].sort((a, b) => (a.storedAt < b.storedAt ? 1 : -1));
	const tasks = new Map<number, Task>();
	const projects = new Map<number, Project>();
	for (const entry of ordered) {
		if (type !== 'projects') {
			const add = (t: unknown): void => {
				if (!looksLikeTask(t) || tasks.has(t.id)) return;
				tasks.set(t.id, t as unknown as Task);
				const embedded = (t as { subtasks?: { items?: unknown } }).subtasks?.items;
				if (Array.isArray(embedded)) embedded.forEach(add);
			};
			add(entry.payload);
			for (const list of candidateLists(entry.payload, entry.path)) list.forEach(add);
		}
		if (type !== 'tasks') {
			for (const list of projectLists(entry)) {
				for (const p of list) {
					if (looksLikeProject(p) && !projects.has(p.id)) projects.set(p.id, p);
				}
			}
		}
	}

	const out: SearchResponse = {};
	if (type !== 'projects') out.tasks = rankAndPage(tasks.values(), q, limit, offset);
	if (type !== 'tasks') out.projects = rankAndPage(projects.values(), q, limit, offset);
	return out;
}
