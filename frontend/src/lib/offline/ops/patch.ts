import type { Task } from '../../api/types';
import { candidateLists, looksLikeTask, type ReadCacheWriter } from '../readCache';

// Cache-patchers for the offline ops' `applyToCache` (FEATURE-OFFLINE-ARCH.md
// §4.5 applyToCache row, §6.2). These rewrite the cached GET records at enqueue
// time so an offline mutation's optimistic result is still present after the app
// is closed and reopened while still offline (a page's in-memory `useListMutator`
// change does NOT survive a restart; the cache does).
//
// The traversal reuses `candidateLists`/`looksLikeTask` from `readCache` and
// passes each entry's stored `path`, so the patchers reach exactly the same task
// arrays `findTask` does — including the ones nested inside aggregates such as
// `/api/v1/config` (`pinnedTasks`, `troiki.*.projects[].tasks`). Missing one of
// those is what made an offline complete of a pinned task silently revert on
// restart. Depending on `readCache` (which the ops already import as a type)
// keeps a single shape definition; it introduces no cycle (`readCache` imports
// nothing from `ops`).

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
		if (mutateTaskInPayload(entry.payload, taskId, mutate, entry.path)) {
			await cache.putEntry(entry);
		}
	}
}

function mutateTaskInPayload(
	payload: unknown,
	taskId: number,
	mutate: TaskMutation,
	path?: string
): boolean {
	let changed = false;
	// A bare Task response (GET /api/v1/tasks/:id).
	if (looksLikeTask(payload) && payload.id === taskId) {
		mutate(payload as Record<string, unknown>);
		changed = true;
	}
	for (const list of candidateLists(payload, path)) {
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

/** The one endpoint whose payload IS an inbox list. */
const INBOX_PATH = '/api/v1/inbox';

/**
 * Path-first: only `/api/v1/inbox` holds an inbox list, so an aggregate that
 * happens to expose top-level `tasks` + `count` + `warnThresholdExceeded` is not
 * mistaken for one (which would push a Task into an unrelated array and bump the
 * wrong counter). Duck-typing survives only as the fallback for an entry whose
 * path is unknown.
 */
function isInboxResponse(payload: unknown, path?: string): payload is InboxResponseShape {
	if (!payload || typeof payload !== 'object') return false;
	if (path !== undefined && path.split('?', 1)[0] !== INBOX_PATH) return false;
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
		if (!isInboxResponse(entry.payload, entry.path)) continue;
		const inbox = entry.payload;
		if (inbox.tasks.some((t) => looksLikeTask(t) && t.id === task.id)) continue;
		inbox.tasks.push(task as unknown as Record<string, unknown>);
		inbox.count += 1;
		await cache.putEntry(entry);
	}
}
