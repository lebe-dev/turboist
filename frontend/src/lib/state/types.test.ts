import { describe, it, expect } from 'vitest';
import type { Task } from '$lib/api/types';
import { taskToFlat, flatToTask, flattenTasks, buildTree, type FlatTask } from './types';

function makeTask(overrides: Partial<Task> = {}): Task {
	return {
		id: '1',
		content: 'Test task',
		description: '',
		project_id: 'proj-1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 4,
		due: null,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2026-01-01T00:00:00Z',
		is_project_task: false,
		children: [],
		...overrides
	};
}

function makeFlat(overrides: Partial<FlatTask> = {}): FlatTask {
	return {
		id: '1',
		content: 'Test task',
		description: '',
		project_id: 'proj-1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 4,
		due_date: null,
		due_recurring: false,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2026-01-01T00:00:00Z',
		is_project_task: false,
		...overrides
	};
}

describe('taskToFlat', () => {
	it('converts task without due date', () => {
		const task = makeTask({ id: 'a', content: 'Hello' });
		const flat = taskToFlat(task);
		expect(flat.id).toBe('a');
		expect(flat.content).toBe('Hello');
		expect(flat.due_date).toBeNull();
		expect(flat.due_recurring).toBe(false);
	});

	it('flattens due object into primitives', () => {
		const task = makeTask({ due: { date: '2026-03-21', recurring: true } });
		const flat = taskToFlat(task);
		expect(flat.due_date).toBe('2026-03-21');
		expect(flat.due_recurring).toBe(true);
	});

	it('preserves nullable fields', () => {
		const task = makeTask({ section_id: 'sec-1', parent_id: 'p-1', completed_at: '2026-01-02' });
		const flat = taskToFlat(task);
		expect(flat.section_id).toBe('sec-1');
		expect(flat.parent_id).toBe('p-1');
		expect(flat.completed_at).toBe('2026-01-02');
	});

	it('copies labels array (no shared reference)', () => {
		const labels = ['a', 'b'];
		const task = makeTask({ labels });
		const flat = taskToFlat(task);
		flat.labels.push('c');
		expect(labels).toEqual(['a', 'b']);
	});
});

describe('flatToTask', () => {
	it('converts flat without due', () => {
		const flat = makeFlat({ id: 'x' });
		const task = flatToTask(flat);
		expect(task.id).toBe('x');
		expect(task.due).toBeNull();
		expect(task.children).toEqual([]);
	});

	it('reconstructs due object from flat fields', () => {
		const flat = makeFlat({ due_date: '2026-05-01', due_recurring: false });
		const task = flatToTask(flat);
		expect(task.due).toEqual({ date: '2026-05-01', recurring: false });
	});

	it('attaches provided children', () => {
		const child = makeTask({ id: 'child' });
		const flat = makeFlat({ id: 'parent' });
		const task = flatToTask(flat, [child]);
		expect(task.children).toHaveLength(1);
		expect(task.children[0].id).toBe('child');
	});
});

describe('flattenTasks', () => {
	it('returns empty array for empty input', () => {
		expect(flattenTasks([])).toEqual([]);
	});

	it('flattens single task without children', () => {
		const tasks = [makeTask({ id: '1' })];
		const flats = flattenTasks(tasks);
		expect(flats).toHaveLength(1);
		expect(flats[0].id).toBe('1');
	});

	it('flattens tree depth-first', () => {
		const tree: Task[] = [
			makeTask({
				id: 'p1',
				children: [
					makeTask({ id: 'c1', parent_id: 'p1' }),
					makeTask({ id: 'c2', parent_id: 'p1' })
				]
			}),
			makeTask({ id: 'p2' })
		];
		const flats = flattenTasks(tree);
		expect(flats.map((f) => f.id)).toEqual(['p1', 'c1', 'c2', 'p2']);
	});

	it('flattens deep nesting', () => {
		const tree: Task[] = [
			makeTask({
				id: 'a',
				children: [
					makeTask({
						id: 'b',
						parent_id: 'a',
						children: [makeTask({ id: 'c', parent_id: 'b' })]
					})
				]
			})
		];
		const flats = flattenTasks(tree);
		expect(flats.map((f) => f.id)).toEqual(['a', 'b', 'c']);
		expect(flats[1].parent_id).toBe('a');
		expect(flats[2].parent_id).toBe('b');
	});
});

describe('buildTree', () => {
	it('returns empty array for empty input', () => {
		expect(buildTree([])).toEqual([]);
	});

	it('builds flat list of roots', () => {
		const flats = [makeFlat({ id: '1' }), makeFlat({ id: '2' })];
		const tree = buildTree(flats);
		expect(tree).toHaveLength(2);
		expect(tree[0].id).toBe('1');
		expect(tree[1].id).toBe('2');
	});

	it('links children to parents', () => {
		const flats = [
			makeFlat({ id: 'p1' }),
			makeFlat({ id: 'c1', parent_id: 'p1' }),
			makeFlat({ id: 'c2', parent_id: 'p1' })
		];
		const tree = buildTree(flats);
		expect(tree).toHaveLength(1);
		expect(tree[0].children).toHaveLength(2);
		expect(tree[0].children[0].id).toBe('c1');
		expect(tree[0].children[1].id).toBe('c2');
	});

	it('builds deep tree', () => {
		const flats = [
			makeFlat({ id: 'a' }),
			makeFlat({ id: 'b', parent_id: 'a' }),
			makeFlat({ id: 'c', parent_id: 'b' })
		];
		const tree = buildTree(flats);
		expect(tree).toHaveLength(1);
		expect(tree[0].children[0].children[0].id).toBe('c');
	});

	it('orphaned children become roots', () => {
		const flats = [makeFlat({ id: 'x', parent_id: 'missing' })];
		const tree = buildTree(flats);
		expect(tree).toHaveLength(1);
		expect(tree[0].id).toBe('x');
	});

	it('roundtrip: flattenTasks → buildTree preserves structure', () => {
		const original: Task[] = [
			makeTask({
				id: 'p1',
				content: 'Parent 1',
				children: [
					makeTask({ id: 'c1', parent_id: 'p1', content: 'Child 1' }),
					makeTask({ id: 'c2', parent_id: 'p1', content: 'Child 2' })
				]
			}),
			makeTask({ id: 'p2', content: 'Parent 2' })
		];
		const flats = flattenTasks(original);
		const rebuilt = buildTree(flats);
		expect(rebuilt).toHaveLength(2);
		expect(rebuilt[0].children).toHaveLength(2);
		expect(rebuilt[0].children[0].content).toBe('Child 1');
		expect(rebuilt[1].children).toHaveLength(0);
	});
});
