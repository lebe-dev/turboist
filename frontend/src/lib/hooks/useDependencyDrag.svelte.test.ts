import { describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';
import {
	refusalsFromCheck,
	useDependencyDrag,
	type DependencyRefusals
} from './useDependencyDrag.svelte';

function task(id: number, parentId: number | null = null, status: Task['status'] = 'open'): Task {
	return { id, parentId, status } as Task;
}

// 1 ─ 2, plus 3 and 4 at the top level.
const tasks = [task(1), task(2, 1), task(3), task(4)];

function setup(refusals: DependencyRefusals = {}) {
	const onDependency = vi.fn();
	const onNest = vi.fn();
	const check = vi.fn(() => Promise.resolve(refusals));
	const drag = useDependencyDrag({ tasks: () => tasks, check, onDependency, onNest });
	return { drag, onDependency, onNest, check };
}

describe('useDependencyDrag', () => {
	it('asks the server about every row the tree allows', () => {
		const { drag, check } = setup();
		drag.begin(3);
		expect(check).toHaveBeenCalledWith(3, [1, 2, 4]);
	});

	it('commits a dependency drop on an acceptable row and resets', () => {
		const { drag, onDependency } = setup();
		drag.begin(3);
		drag.over(4, 'dependency', 10, 20);
		expect(drag.hover).toEqual({ targetId: 4, mode: 'dependency', x: 10, y: 20 });
		drag.drop(4, 'dependency');
		expect(onDependency).toHaveBeenCalledWith(3, 4);
		expect(drag.draggedId).toBeNull();
		expect(drag.hover).toBeNull();
	});

	it('refuses a structural dependency target without committing', () => {
		const { drag, onDependency } = setup();
		drag.begin(2);
		expect(drag.refusalFor(1, 'dependency')).toBe('ancestor');
		drag.drop(1, 'dependency');
		expect(onDependency).not.toHaveBeenCalled();
	});

	it('applies the server refusals once they arrive', async () => {
		const { drag, onDependency } = setup({ 4: 'cycle' });
		drag.begin(3);
		expect(drag.refusalFor(4, 'dependency')).toBeNull();
		await Promise.resolve();
		await Promise.resolve();
		expect(drag.refusalFor(4, 'dependency')).toBe('cycle');
		drag.drop(4, 'dependency');
		expect(onDependency).not.toHaveBeenCalled();
	});

	it('drops a late answer that belongs to an earlier gesture', async () => {
		let resolveFirst: (m: DependencyRefusals) => void = () => undefined;
		const check = vi
			.fn()
			.mockImplementationOnce(() => new Promise((r) => (resolveFirst = r)))
			.mockImplementation(() => Promise.resolve({}));
		const drag = useDependencyDrag({ tasks: () => tasks, check, onDependency: vi.fn(), onNest: vi.fn() });
		drag.begin(3);
		drag.cancel();
		drag.begin(3);
		resolveFirst({ 4: 'exists' });
		await Promise.resolve();
		await Promise.resolve();
		expect(drag.refusalFor(4, 'dependency')).toBeNull();
	});

	it('hides the hover over the dragged row itself and ignores a drop there', () => {
		const { drag, onDependency } = setup();
		drag.begin(3);
		drag.over(3, 'dependency', 1, 1);
		expect(drag.hover).toBeNull();
		drag.drop(3, 'dependency');
		expect(onDependency).not.toHaveBeenCalled();
	});

	it('keeps working when the check fails', async () => {
		const onDependency = vi.fn();
		const drag = useDependencyDrag({
			tasks: () => tasks,
			check: () => Promise.reject(new Error('offline')),
			onDependency,
			onNest: vi.fn()
		});
		drag.begin(3);
		await Promise.resolve();
		drag.drop(4, 'dependency');
		expect(onDependency).toHaveBeenCalledWith(3, 4);
	});

	it('commits a nest drop on an acceptable row', () => {
		const { drag, onNest } = setup();
		drag.begin(4);
		drag.over(3, 'nest', 5, 5);
		expect(drag.hover).toEqual({ targetId: 3, mode: 'nest', x: 5, y: 5 });
		drag.drop(3, 'nest');
		expect(onNest).toHaveBeenCalledWith(4, 3);
	});

	it('refuses nesting a task under its own descendant', () => {
		const { drag, onNest } = setup();
		drag.begin(1);
		expect(drag.refusalFor(2, 'nest')).toBe('descendant');
		drag.drop(2, 'nest');
		expect(onNest).not.toHaveBeenCalled();
	});

	it('does not offer nesting under the row that is already the parent', () => {
		const { drag, onNest } = setup();
		drag.begin(2);
		drag.over(1, 'nest', 1, 1);
		expect(drag.hover).toBeNull();
		drag.drop(1, 'nest');
		expect(onNest).not.toHaveBeenCalled();
	});

	it('never mixes up which callback a mode dispatches to', () => {
		const { drag, onDependency, onNest } = setup();
		drag.begin(3);
		drag.drop(4, 'nest');
		expect(onNest).toHaveBeenCalledWith(3, 4);
		expect(onDependency).not.toHaveBeenCalled();
	});
});

describe('refusalsFromCheck', () => {
	it('keeps only the refusals a row can show', () => {
		const m = refusalsFromCheck([
			{ taskId: 1, reason: 'relation_exists' },
			{ taskId: 2, reason: 'not_found' },
			{ taskId: 3, reason: 'relation_cycle' }
		]);
		expect(m).toEqual({ 1: 'exists', 3: 'cycle' });
	});
});
