import type { Priority } from '../api/types';

export const PRIORITY_ORDER: Priority[] = ['high', 'medium', 'low', 'no-priority'];

export const PRIORITY_LABEL: Record<Priority, string> = {
	high: 'P1',
	medium: 'P2',
	low: 'P3',
	'no-priority': 'P4'
};

/**
 * Tailwind text colour classes for priority dots. P4 stays muted because
 * "no priority" should not draw the eye.
 */
export const PRIORITY_COLOR: Record<Priority, string> = {
	high: 'text-red-500',
	medium: 'text-amber-500',
	low: 'text-blue-500',
	'no-priority': 'text-muted-foreground'
};

export function comparePriority(a: Priority, b: Priority): number {
	return PRIORITY_ORDER.indexOf(a) - PRIORITY_ORDER.indexOf(b);
}

interface TaskOrderable {
	id: number;
	priority: Priority;
	isPinned: boolean;
	pinnedAt: string | null;
	createdAt: string;
}

/**
 * Mirrors the backend `taskOrderBy` (repo/tasks.go): pinned first, then by
 * priority (P1 → P4), then most recently pinned, then most recently created.
 * `id` breaks ties so the order stays stable when timestamps collide.
 */
export function compareTaskOrder(a: TaskOrderable, b: TaskOrderable): number {
	if (a.isPinned !== b.isPinned) return a.isPinned ? -1 : 1;
	const byPriority = comparePriority(a.priority, b.priority);
	if (byPriority !== 0) return byPriority;
	if (a.isPinned && b.isPinned) {
		const ap = a.pinnedAt ?? '';
		const bp = b.pinnedAt ?? '';
		if (ap !== bp) return bp.localeCompare(ap);
	}
	if (a.createdAt !== b.createdAt) return b.createdAt.localeCompare(a.createdAt);
	return b.id - a.id;
}
