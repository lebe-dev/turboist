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
