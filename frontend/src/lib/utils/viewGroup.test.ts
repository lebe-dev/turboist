import { describe, it, expect } from 'vitest';
import { groupByDayPart, groupByDay } from './viewGroup';
import type { Task } from '../api/types';

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
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		recurrenceRule: null,
		labels: [],
		url: '',
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

describe('groupByDayPart', () => {
	it('groups in canonical order and skips empty buckets', () => {
		const tasks = [
			makeTask({ id: 1, dayPart: 'evening' }),
			makeTask({ id: 2, dayPart: 'morning' }),
			makeTask({ id: 3, dayPart: 'morning' }),
			makeTask({ id: 4, dayPart: 'none' })
		];
		const groups = groupByDayPart(tasks);
		expect(groups.map((g) => g.part)).toEqual(['morning', 'evening', 'none']);
		expect(groups[0].tasks.map((t) => t.id)).toEqual([2, 3]);
	});

	it('returns empty array for empty input', () => {
		expect(groupByDayPart([])).toEqual([]);
	});
});

describe('groupByDay', () => {
	it('buckets by local calendar day, sorted ascending', () => {
		const today = new Date();
		today.setHours(10, 0, 0, 0);
		const tomorrow = new Date(today);
		tomorrow.setDate(tomorrow.getDate() + 1);
		const tasks = [
			makeTask({ id: 1, dueAt: tomorrow.toISOString() }),
			makeTask({ id: 2, dueAt: today.toISOString() }),
			makeTask({ id: 3, dueAt: today.toISOString() })
		];
		const groups = groupByDay(tasks);
		expect(groups).toHaveLength(2);
		expect(groups[0].label).toBe('Today');
		expect(groups[0].tasks.map((t) => t.id).sort()).toEqual([2, 3]);
		expect(groups[1].label).toBe('Tomorrow');
	});

	it('places dateless tasks into a No date bucket at the end', () => {
		const today = new Date();
		const tasks = [
			makeTask({ id: 1, dueAt: null }),
			makeTask({ id: 2, dueAt: today.toISOString() })
		];
		const groups = groupByDay(tasks);
		expect(groups[groups.length - 1].dayKey).toBe('no-date');
		expect(groups[groups.length - 1].tasks.map((t) => t.id)).toEqual([1]);
	});
});
