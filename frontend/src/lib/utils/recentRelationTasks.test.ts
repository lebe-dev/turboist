import { describe, expect, it } from 'vitest';
import type { RecentEntry } from '$lib/utils/quickSwitcher';
import { recentRelationTasks } from './recentRelationTasks';

const task = (id: number): RecentEntry => ({ kind: 'task', id, title: `T${id}`, labels: [] });
const project = (id: number): RecentEntry => ({ kind: 'project', id, title: `P${id}`, labels: [] });

describe('recentRelationTasks', () => {
	it('keeps only tasks, newest first', () => {
		const out = recentRelationTasks([task(1), project(2), task(3)], new Set());
		expect(out.map((e) => e.id)).toEqual([1, 3]);
	});

	it('drops excluded ids before cutting to five', () => {
		const entries = [1, 2, 3, 4, 5, 6, 7].map(task);
		const out = recentRelationTasks(entries, new Set([1, 2]));
		expect(out.map((e) => e.id)).toEqual([3, 4, 5, 6, 7]);
	});
});
