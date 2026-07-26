import type { Task } from '../../api/types';
import { candidateLists, looksLikeTask, type ReadCacheWriter } from '../readCache';

// Cache-patchers for the offline ops' `applyToCache` (FEATURE-OFFLINE-ARCH.md
// §4.5 applyToCache row, §6.2). These rewrite the cached GET records at enqueue
// time so an offline mutation's optimistic result is still present after the app
// is closed and reopened while still offline (a page's in-memory `useListMutator`
// change does NOT survive a restart; the cache does).
//
// The traversal reuses `candidateLists`/`looksLikeTask` from `readCache`, so the
// patchers touch exactly the same response shapes as `findTask` (Page/ViewList
// `.items`, `InboxResponse.tasks[]`, ProjectBundle/SearchResponse `.tasks.items`,
// TodayBundle `today`/`overdue`/`completedToday`, and a bare Task). Depending on
// `readCache` (which the ops already import as a type) keeps a single shape
// definition; it introduces no cycle (`readCache` imports nothing from `ops`).

/** Mutation applied in place to each cached copy of a task matched by id. */
type TaskMutation = (task: Record<string, unknown>) => void;

/**
 * Patch every cached copy of task `taskId` in place and rewrite only the entries
 * that actually contained it (so unrelated cache rows are not churned). Used by
 * `task.complete` / `task.uncomplete` to flip status + `completedAt` across all
 * cached lists at once.
 */
export async function patchCachedTask(
	cache: ReadCacheWriter,
	taskId: number,
	mutate: TaskMutation
): Promise<void> {
	for (const entry of await cache.getAll()) {
		if (mutateTaskInPayload(entry.payload, taskId, mutate)) {
			await cache.putEntry(entry);
		}
	}
}

function mutateTaskInPayload(payload: unknown, taskId: number, mutate: TaskMutation): boolean {
	let changed = false;
	// A bare Task response (GET /api/v1/tasks/:id).
	if (looksLikeTask(payload) && payload.id === taskId) {
		mutate(payload as Record<string, unknown>);
		changed = true;
	}
	for (const list of candidateLists(payload)) {
		for (const item of list) {
			if (looksLikeTask(item) && item.id === taskId) {
				mutate(item as Record<string, unknown>);
				changed = true;
			}
		}
	}
	return changed;
}

/** InboxResponse — the only shape `task.createInbox` inserts into (§4.5). */
interface InboxResponseShape {
	count: number;
	warnThresholdExceeded: boolean;
	tasks: Record<string, unknown>[];
}

function isInboxResponse(payload: unknown): payload is InboxResponseShape {
	if (!payload || typeof payload !== 'object') return false;
	const p = payload as Record<string, unknown>;
	return Array.isArray(p.tasks) && 'count' in p && 'warnThresholdExceeded' in p;
}

/**
 * Insert an offline-created `task` into every cached inbox-list record and bump
 * its `count`, so the tmp task is on the Inbox after a restart. Idempotent: a
 * record that already holds the same id (e.g. a re-applied op) is left untouched.
 */
export async function insertTaskIntoInboxCache(
	cache: ReadCacheWriter,
	task: Task
): Promise<void> {
	for (const entry of await cache.getAll()) {
		if (!isInboxResponse(entry.payload)) continue;
		const inbox = entry.payload;
		if (inbox.tasks.some((t) => looksLikeTask(t) && t.id === task.id)) continue;
		inbox.tasks.push(task as unknown as Record<string, unknown>);
		inbox.count += 1;
		await cache.putEntry(entry);
	}
}
