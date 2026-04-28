import { describe, expect, it } from 'vitest';
import { buildTree, flattenTree } from './taskTree';
import type { Task } from '../api/types';

function task(id: number, parentId: number | null = null, title = `t${id}`): Task {
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
		status: 'open',
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
