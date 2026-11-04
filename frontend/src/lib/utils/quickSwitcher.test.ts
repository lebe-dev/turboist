import { describe, expect, it } from 'vitest';
import {
	foldCase,
	matchRank,
	switcherResults,
	todayTargetProjects,
	type RecentEntry
} from './quickSwitcher';
import type { Label, Project, Task } from '$lib/api/types';

function label(name: string): Label {
	return { id: name.length, name } as Label;
}

function task(id: number, title: string, extra: Partial<Task> = {}): Task {
	return { id, title, description: '', labels: [], status: 'open', ...extra } as Task;
}

function project(id: number, title: string, extra: Partial<Project> = {}): Project {
	return { id, title, description: '', labels: [], status: 'open', ...extra } as Project;
}

describe('foldCase', () => {
	it('folds Cyrillic and Latin alike', () => {
		expect(foldCase('ОТПУСК Plan')).toBe('отпуск plan');
	});
});

describe('matchRank', () => {
	it('ranks a title hit above a label hit above a description hit', () => {
		expect(matchRank('отп', { title: 'Отпуск', labels: [], description: '' })).toBe(0);
		expect(matchRank('сад', { title: 'Семена', labels: ['Сад'], description: '' })).toBe(1);
		expect(matchRank('fence', { title: 'Call', labels: [], description: 'the fence' })).toBe(2);
	});

	it('returns null when nothing matches or the query is blank', () => {
		expect(matchRank('zzz', { title: 'Call', labels: ['x'], description: 'y' })).toBeNull();
		expect(matchRank('   ', { title: 'Call', labels: [], description: '' })).toBeNull();
	});
});

describe('switcherResults', () => {
	const recents: RecentEntry[] = [
		{ kind: 'task', id: 10, title: 'Garden fence', labels: [] },
		{ kind: 'project', id: 20, title: 'Garden', labels: [] },
		{ kind: 'task', id: 11, title: 'Call mom', labels: [] }
	];

	it('shows only the recents, in recency order, for an empty query', () => {
		const out = switcherResults('', recents, [], []);
		expect(out.map((r) => `${r.kind}:${r.id}`)).toEqual(['task:10', 'project:20', 'task:11']);
		expect(out.every((r) => r.recent)).toBe(true);
	});

	it('puts matching recents first, then everything else by match rank', () => {
		const tasks = [
			task(30, 'Buy seeds', { labels: [label('garden')] }),
			task(10, 'Garden fence'),
			task(31, 'Garden plan')
		];
		const projects = [project(40, 'Dacha', { labels: [label('garden')] }), project(20, 'Garden')];
		const out = switcherResults('garden', recents, tasks, projects);
		expect(out.map((r) => `${r.kind}:${r.id}`)).toEqual([
			'task:10',
			'project:20',
			// rank 0 (title) — projects ahead of tasks
			'task:31',
			// rank 1 (label)
			'project:40',
			'task:30'
		]);
		expect(out.slice(0, 2).every((r) => r.recent)).toBe(true);
		expect(out.slice(2).some((r) => r.recent)).toBe(false);
	});

	it('keeps a matching recent the search has not returned (offline, or not yet answered)', () => {
		const out = switcherResults('mom', recents, [], []);
		expect(out.map((r) => `${r.kind}:${r.id}`)).toEqual(['task:11']);
	});

	it('prefers the fresh search copy over the remembered snapshot', () => {
		const out = switcherResults('garden', recents, [task(10, 'Garden fence v2')], []);
		expect(out[0]).toMatchObject({ kind: 'task', id: 10, title: 'Garden fence v2' });
	});

	it('drops a recent that no longer matches', () => {
		const out = switcherResults('mom', recents, [], []);
		expect(out.find((r) => r.id === 10)).toBeUndefined();
	});
});

describe('todayTargetProjects', () => {
	const projects = [
		{ id: 1, status: 'open' },
		{ id: 2, status: 'completed' },
		{ id: 3, status: 'open' },
		{ id: 4, status: 'open' }
	];

	it('picks the two most recent open projects that still exist', () => {
		expect(todayTargetProjects([9, 2, 3, 1, 4], projects).map((p) => p.id)).toEqual([3, 1]);
	});

	it('returns fewer, or none, when fewer qualify', () => {
		expect(todayTargetProjects([2, 4], projects).map((p) => p.id)).toEqual([4]);
		expect(todayTargetProjects([9, 2], projects)).toEqual([]);
	});
});
