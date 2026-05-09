import { describe, it, expect, vi, afterEach } from 'vitest';
import { activeDayPart, groupByCompletedDay, groupByDay, groupByDayPart } from './viewGroup';
import type { ConfigResponse, Task } from '../api/types';

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
		isPrivate: false,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
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
	afterEach(() => vi.useRealTimers());

	it('buckets by local calendar day, sorted ascending', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-15T10:00:00.000Z'));
		const today = new Date('2026-01-15T10:00:00.000Z');
		const tomorrow = new Date('2026-01-16T10:00:00.000Z');
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
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-15T10:00:00.000Z'));
		const today = new Date('2026-01-15T10:00:00.000Z');
		const tasks = [
			makeTask({ id: 1, dueAt: null }),
			makeTask({ id: 2, dueAt: today.toISOString() })
		];
		const groups = groupByDay(tasks);
		expect(groups[groups.length - 1].dayKey).toBe('no-date');
		expect(groups[groups.length - 1].tasks.map((t) => t.id)).toEqual([1]);
	});
});

describe('groupByCompletedDay', () => {
	afterEach(() => vi.useRealTimers());

	it('skips tasks without completedAt', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-15T10:00:00.000Z'));
		const tasks = [
			makeTask({ id: 1, completedAt: null }),
			makeTask({ id: 2, completedAt: '2026-01-15T08:00:00.000Z' })
		];
		const groups = groupByCompletedDay(tasks, 'UTC');
		expect(groups).toHaveLength(1);
		expect(groups[0].tasks.map((t) => t.id)).toEqual([2]);
	});

	it('orders groups by most recent first and labels Today/Yesterday', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-15T10:00:00.000Z'));
		const tasks = [
			makeTask({ id: 1, completedAt: '2026-01-13T08:00:00.000Z' }),
			makeTask({ id: 2, completedAt: '2026-01-15T08:00:00.000Z' }),
			makeTask({ id: 3, completedAt: '2026-01-14T08:00:00.000Z' })
		];
		const groups = groupByCompletedDay(tasks, 'UTC');
		expect(groups.map((g) => g.dayKey)).toEqual(['2026-01-15', '2026-01-14', '2026-01-13']);
		expect(groups[0].label).toBe('Today');
		expect(groups[1].label).toBe('Yesterday');
		expect(groups[2].label).toMatch(/[A-Z][a-z]{2}/); // weekday-month-day
	});

	it('returns empty array when no tasks have completedAt', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-15T10:00:00.000Z'));
		expect(groupByCompletedDay([], 'UTC')).toEqual([]);
		expect(groupByCompletedDay([makeTask({ completedAt: null })], 'UTC')).toEqual([]);
	});
});

describe('activeDayPart', () => {
	const dayParts: ConfigResponse['dayParts'] = {
		morning: { start: 6, end: 12 },
		afternoon: { start: 12, end: 18 },
		evening: { start: 18, end: 22 }
	};

	it('returns null when dayParts config is missing', () => {
		expect(activeDayPart(new Date('2026-04-28T08:00:00.000Z'), undefined, 'UTC')).toBeNull();
	});

	it('selects morning/afternoon/evening based on the hour in the timezone', () => {
		expect(activeDayPart(new Date('2026-04-28T08:00:00.000Z'), dayParts, 'UTC')).toBe('morning');
		expect(activeDayPart(new Date('2026-04-28T13:00:00.000Z'), dayParts, 'UTC')).toBe('afternoon');
		expect(activeDayPart(new Date('2026-04-28T20:00:00.000Z'), dayParts, 'UTC')).toBe('evening');
	});

	it('returns null outside any defined interval', () => {
		expect(activeDayPart(new Date('2026-04-28T03:00:00.000Z'), dayParts, 'UTC')).toBeNull();
		expect(activeDayPart(new Date('2026-04-28T23:00:00.000Z'), dayParts, 'UTC')).toBeNull();
	});

	it('respects the timezone parameter', () => {
		// 04:00 UTC = 08:00 in Tbilisi (UTC+4) → morning, but 04:00 UTC itself is outside.
		const instant = new Date('2026-04-28T04:00:00.000Z');
		expect(activeDayPart(instant, dayParts, 'Asia/Tbilisi')).toBe('morning');
		expect(activeDayPart(instant, dayParts, 'UTC')).toBeNull();
	});
});
