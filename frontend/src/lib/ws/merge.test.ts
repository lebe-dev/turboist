import { describe, it, expect } from 'vitest';
import { mergeUpserted, filterByIds } from './merge';
import type { Task } from '$lib/api/types';

function makeTask(id: string, content = `Task ${id}`, children: Task[] = []): Task {
	return {
		id,
		content,
		description: '',
		project_id: 'p1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: children.length,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2024-01-01T00:00:00Z',
		is_project_task: false,
		postpone_count: 0,
		children
	};
}

describe('mergeUpserted', () => {
	it('returns existing when upserted is empty', () => {
		const existing = [makeTask('1'), makeTask('2')];
		const result = mergeUpserted(existing, []);
		expect(result).toBe(existing);
	});

	it('replaces existing task by id', () => {
		const existing = [makeTask('1', 'old'), makeTask('2')];
		const upserted = [makeTask('1', 'new')];
		const result = mergeUpserted(existing, upserted);

		expect(result).toHaveLength(2);
		expect(result[0].content).toBe('new');
		expect(result[1].id).toBe('2');
	});

	it('appends new tasks not in existing', () => {
		const existing = [makeTask('1')];
		const upserted = [makeTask('2')];
		const result = mergeUpserted(existing, upserted);

		expect(result).toHaveLength(2);
		expect(result[0].id).toBe('1');
		expect(result[1].id).toBe('2');
	});

	it('handles mix of replacements and appends', () => {
		const existing = [makeTask('1', 'old'), makeTask('2')];
		const upserted = [makeTask('1', 'updated'), makeTask('3', 'new')];
		const result = mergeUpserted(existing, upserted);

		expect(result).toHaveLength(3);
		expect(result[0].content).toBe('updated');
		expect(result[1].id).toBe('2');
		expect(result[2].id).toBe('3');
	});

	it('preserves order of existing tasks', () => {
		const existing = [makeTask('a'), makeTask('b'), makeTask('c')];
		const upserted = [makeTask('b', 'B updated')];
		const result = mergeUpserted(existing, upserted);

		expect(result.map((t) => t.id)).toEqual(['a', 'b', 'c']);
		expect(result[1].content).toBe('B updated');
	});
});

describe('filterByIds', () => {
	it('returns original when removeIds is empty', () => {
		const tasks = [makeTask('1'), makeTask('2')];
		const result = filterByIds(tasks, []);
		expect(result).toBe(tasks);
	});

	it('removes root-level tasks by id', () => {
		const tasks = [makeTask('1'), makeTask('2'), makeTask('3')];
		const result = filterByIds(tasks, ['2']);

		expect(result).toHaveLength(2);
		expect(result.map((t) => t.id)).toEqual(['1', '3']);
	});

	it('removes nested children by id', () => {
		const child = makeTask('child-1');
		const parent = makeTask('parent', 'Parent', [child, makeTask('child-2')]);
		const tasks = [parent];

		const result = filterByIds(tasks, ['child-1']);

		expect(result).toHaveLength(1);
		expect(result[0].children).toHaveLength(1);
		expect(result[0].children[0].id).toBe('child-2');
	});

	it('removes deeply nested tasks', () => {
		const grandchild = makeTask('gc');
		const child = makeTask('c', 'Child', [grandchild]);
		const parent = makeTask('p', 'Parent', [child]);

		const result = filterByIds([parent], ['gc']);

		expect(result[0].children[0].children).toHaveLength(0);
	});

	it('removes multiple tasks at different levels', () => {
		const child = makeTask('c1');
		const parent = makeTask('p1', 'Parent', [child, makeTask('c2')]);
		const tasks = [parent, makeTask('root2')];

		const result = filterByIds(tasks, ['c1', 'root2']);

		expect(result).toHaveLength(1);
		expect(result[0].id).toBe('p1');
		expect(result[0].children).toHaveLength(1);
		expect(result[0].children[0].id).toBe('c2');
	});
});
