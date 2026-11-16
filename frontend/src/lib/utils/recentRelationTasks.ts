import type { RecentEntry } from '$lib/utils/quickSwitcher';

/** How many recently opened tasks the link dialog offers before anything is typed. */
export const RECENT_RELATION_TASKS = 5;

/**
 * The most recently opened tasks, newest first, for the link picker.
 *
 * Projects are skipped, and so are the ids in `excluded` (the task itself and
 * its existing peers) — both before the cut, so the list stays full-length.
 */
export function recentRelationTasks(
	entries: readonly RecentEntry[],
	excluded: ReadonlySet<number>,
	limit: number = RECENT_RELATION_TASKS
): RecentEntry[] {
	return entries.filter((e) => e.kind === 'task' && !excluded.has(e.id)).slice(0, limit);
}
