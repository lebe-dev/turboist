import { describe, expect, it } from 'vitest';
import { buildTree, flattenTree, splitByRootCompletion } from './taskTree';
import type { Task, TaskStatus } from '../api/types';

function task(
	id: number,
	parentId: number | null = null,
	status: TaskStatus = 'open',
	title = `t${id}`
): Task {
	return {
		id,
		title,
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId,
		priority: 'no-priority',
		status,
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: ''
	};
}

describe('buildTree', () => {
	it('returns empty roots for empty input', () => {
		expect(buildTree([])).toEqual([]);
	});

	it('builds a flat tree when all tasks are roots', () => {
		const tree = buildTree([task(1), task(2), task(3)]);
		expect(tree.map((n) => n.task.id)).toEqual([1, 2, 3]);
		expect(tree.every((n) => n.children.length === 0)).toBe(true);
	});

	it('nests children under parents and preserves input order', () => {
		const tree = buildTree([task(1), task(2, 1), task(3, 1), task(4, 2), task(5)]);
		expect(tree.map((n) => n.task.id)).toEqual([1, 5]);
		expect(tree[0].children.map((n) => n.task.id)).toEqual([2, 3]);
		expect(tree[0].children[0].children.map((n) => n.task.id)).toEqual([4]);
	});

	it('treats orphaned children (unknown parent) as roots', () => {
		const tree = buildTree([task(2, 99), task(3)]);
		expect(tree.map((n) => n.task.id)).toEqual([2, 3]);
	});
});

describe('flattenTree', () => {
	it('returns tasks in depth-first order', () => {
		const tree = buildTree([task(1), task(2, 1), task(3, 2), task(4, 1), task(5)]);
		expect(flattenTree(tree).map((t) => t.id)).toEqual([1, 2, 3, 4, 5]);
	});
});

describe('splitByRootCompletion', () => {
	it('moves descendants of a completed root into done', () => {
		const items = [
			task(1, null, 'completed'),
			task(2, 1, 'open'), // child of completed root
			task(3, 2, 'open') // grand-child of completed root
		];
		const { open, done } = splitByRootCompletion(items);
		expect(open.map((t) => t.id)).toEqual([]);
		expect(done.map((t) => t.id)).toEqual([1, 2, 3]);
	});

	it('keeps completed children of an open root in open', () => {
		const items = [
			task(1, null, 'open'),
			task(2, 1, 'completed'), // completed child under open root → still open bucket
			task(3, null, 'open')
		];
		const { open, done } = splitByRootCompletion(items);
		expect(open.map((t) => t.id)).toEqual([1, 2, 3]);
		expect(done).toEqual([]);
	});

	it('treats tasks with unknown parentId as roots', () => {
		const items = [
			task(2, 99, 'completed'), // parent 99 not in input → 2 is its own root
			task(3, 99, 'open') // parent 99 not in input → 3 is its own root
		];
		const { open, done } = splitByRootCompletion(items);
		expect(open.map((t) => t.id)).toEqual([3]);
		expect(done.map((t) => t.id)).toEqual([2]);
	});

	it('returns empty buckets for empty input', () => {
		expect(splitByRootCompletion([])).toEqual({ open: [], done: [] });
	});
});
