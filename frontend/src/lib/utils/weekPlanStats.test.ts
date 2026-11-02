import { describe, it, expect } from 'vitest';
import { weekPlanStats } from './weekPlanStats';
import type { Label, Project, Task, TroikiCategory } from '../api/types';

function makeLabel(id: number, name: string): Label {
	return {
		id,
		name,
		color: 'blue',
		description: '',
		isFavourite: false,
		isPrivate: false,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z'
	};
}

function makeProject(id: number, troikiCategory: TroikiCategory | null): Project {
	return {
		id,
		contextId: 1,
		title: `p${id}`,
		description: '',
		color: 'blue',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z'
	} as Project;
}

function makeTask(overrides: Partial<Task>): Task {
	return {
		id: 1,
		title: 't',
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'week',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		autoSortedAt: null,
		autoSortUndecidedAt: null,
		blockedByCount: 0,
		relationCount: 0,
		labels: [],
		url: '',
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

describe('weekPlanStats', () => {
	const work = makeLabel(1, 'work');
	const home = makeLabel(2, 'home');
	const admin = makeLabel(3, 'admin');

	it('orders labels by usage, then by name', () => {
		const stats = weekPlanStats(
			[
				makeTask({ id: 1, labels: [home, work] }),
				makeTask({ id: 2, labels: [work] }),
				makeTask({ id: 3, labels: [admin] }),
				makeTask({ id: 4, labels: [home, work] })
			],
			[]
		);
		expect(stats.labels.map((u) => [u.label.name, u.count])).toEqual([
			['work', 3],
			['home', 2],
			['admin', 1]
		]);
		expect(stats.total).toBe(4);
	});

	it('counts tasks whose project stands in a troiki bucket', () => {
		const stats = weekPlanStats(
			[
				makeTask({ id: 1, projectId: 10 }),
				makeTask({ id: 2, projectId: 11 }),
				makeTask({ id: 3, projectId: 12, parentId: 1 }),
				makeTask({ id: 4, projectId: null })
			],
			[makeProject(10, 'important'), makeProject(11, null), makeProject(12, 'rest')]
		);
		expect(stats.fromTroiki).toBe(2);
	});

	it('ignores tasks that are no longer open', () => {
		const stats = weekPlanStats(
			[
				makeTask({ id: 1, labels: [work], projectId: 10 }),
				makeTask({ id: 2, labels: [work], projectId: 10, status: 'completed' })
			],
			[makeProject(10, 'medium')]
		);
		expect(stats.labels).toEqual([{ label: work, count: 1 }]);
		expect(stats.fromTroiki).toBe(1);
		expect(stats.total).toBe(1);
	});

	it('is empty for an empty plan', () => {
		expect(weekPlanStats([], [])).toEqual({ labels: [], fromTroiki: 0, total: 0 });
	});
});
