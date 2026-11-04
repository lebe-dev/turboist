import { describe, expect, it } from 'vitest';
import { matchSearchPath, searchFromCache } from './searchFallback';
import type { CachedResponse } from './db';

function task(id: number, title: string, extra: Record<string, unknown> = {}) {
	return {
		id,
		title,
		description: '',
		labels: [],
		status: 'open',
		dayPart: 'none',
		planState: 'none',
		...extra
	};
}

function project(id: number, title: string, extra: Record<string, unknown> = {}) {
	return { id, title, description: '', labels: [], status: 'open', ...extra };
}

function entry(path: string, payload: unknown, storedAt = '2026-11-01T00:00:00.000Z'): CachedResponse {
	return { cacheKey: path, path, payload, storedAt };
}

describe('matchSearchPath', () => {
	it('recognises the search endpoint only', () => {
		expect(matchSearchPath('/api/v1/search')).toBe(true);
		expect(matchSearchPath('/api/v1/search?q=x')).toBe(true);
		expect(matchSearchPath('/api/v1/tasks')).toBe(false);
	});
});

describe('searchFromCache', () => {
	const entries: CachedResponse[] = [
		entry('/api/v1/tasks/today', {
			today: [task(1, 'Отпуск: билеты'), task(2, 'Buy seeds', { labels: [{ id: 9, name: 'Отпуск' }] })],
			overdue: [],
			completedToday: [task(3, 'Отпуск: виза', { status: 'completed' })]
		}),
		entry('/api/v1/inbox', { count: 1, tasks: [task(4, 'Call', { description: 'про отпуск' })] }),
		entry('/api/v1/config', {
			projects: [project(10, 'Отпуск 2027'), project(11, 'Dacha')],
			pinnedTasks: []
		})
	];

	it('ranks title hits, then labels, then descriptions, open before finished', () => {
		const res = searchFromCache(entries, { q: 'ОТПУСК', type: 'all' });
		expect(res.tasks?.items.map((t) => t.id)).toEqual([1, 3, 2, 4]);
		expect(res.tasks?.total).toBe(4);
		expect(res.projects?.items.map((p) => p.id)).toEqual([10]);
	});

	it('honours type and paging', () => {
		const res = searchFromCache(entries, { q: 'отпуск', type: 'tasks', limit: 2, offset: 1 });
		expect(res.projects).toBeUndefined();
		expect(res.tasks?.items.map((t) => t.id)).toEqual([3, 2]);
		expect(res.tasks).toMatchObject({ total: 4, limit: 2, offset: 1 });
	});

	it('keeps the freshest copy of a task cached under several endpoints', () => {
		const res = searchFromCache(
			[
				entry('/api/v1/inbox', { tasks: [task(5, 'Old title plan')] }, '2026-11-01T00:00:00.000Z'),
				entry('/api/v1/tasks/today', { today: [task(5, 'New title plan')] }, '2026-11-02T00:00:00.000Z')
			],
			{ q: 'plan', type: 'tasks' }
		);
		expect(res.tasks?.items).toHaveLength(1);
		expect(res.tasks?.items[0].title).toBe('New title plan');
	});
});
