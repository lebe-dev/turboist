import { describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';
import {
	refusalsFromCheck,
	useDependencyDrag,
	type DependencyRefusals
} from './useDependencyDrag.svelte';

function task(id: number, parentId: number | null = null): Task {
	return { id, parentId, status: 'open' } as Task;
}

// 1 ─ 2, plus 3 and 4 at the top level.
const tasks = [task(1), task(2, 1), task(3), task(4)];

function setup(refusals: DependencyRefusals = {}) {
	const onDrop = vi.fn();
	const check = vi.fn(() => Promise.resolve(refusals));
	const drag = useDependencyDrag({ tasks: () => tasks, check, onDrop });
	return { drag, onDrop, check };
}

describe('useDependencyDrag', () => {
	it('asks the server about every row the tree allows', () => {
		const { drag, check } = setup();
		drag.begin(3);
		expect(check).toHaveBeenCalledWith(3, [1, 2, 4]);
	});

	it('commits a drop on an acceptable row and resets', () => {
		const { drag, onDrop } = setup();
		drag.begin(3);
		drag.over(4, 10, 20);
		expect(drag.hover).toEqual({ targetId: 4, x: 10, y: 20 });
		drag.drop(4);
		expect(onDrop).toHaveBeenCalledWith(3, 4);
		expect(drag.draggedId).toBeNull();
		expect(drag.hover).toBeNull();
	});

	it('refuses a structural target without committing', () => {
		const { drag, onDrop } = setup();
		drag.begin(2);
		expect(drag.refusalFor(1)).toBe('ancestor');
		drag.drop(1);
		expect(onDrop).not.toHaveBeenCalled();
	});

	it('applies the server refusals once they arrive', async () => {
		const { drag, onDrop } = setup({ 4: 'cycle' });
		drag.begin(3);
		expect(drag.refusalFor(4)).toBeNull();
		await Promise.resolve();
		await Promise.resolve();
		expect(drag.refusalFor(4)).toBe('cycle');
		drag.drop(4);
		expect(onDrop).not.toHaveBeenCalled();
	});

	it('drops a late answer that belongs to an earlier gesture', async () => {
		let resolveFirst: (m: DependencyRefusals) => void = () => undefined;
		const check = vi
			.fn()
			.mockImplementationOnce(() => new Promise((r) => (resolveFirst = r)))
			.mockImplementation(() => Promise.resolve({}));
		const drag = useDependencyDrag({ tasks: () => tasks, check, onDrop: vi.fn() });
		drag.begin(3);
		drag.cancel();
		drag.begin(3);
		resolveFirst({ 4: 'exists' });
		await Promise.resolve();
		await Promise.resolve();
		expect(drag.refusalFor(4)).toBeNull();
	});

	it('hides the hover over the dragged row itself and ignores a drop there', () => {
		const { drag, onDrop } = setup();
		drag.begin(3);
		drag.over(3, 1, 1);
		expect(drag.hover).toBeNull();
		drag.drop(3);
		expect(onDrop).not.toHaveBeenCalled();
	});

	it('keeps working when the check fails', async () => {
		const onDrop = vi.fn();
		const drag = useDependencyDrag({
			tasks: () => tasks,
			check: () => Promise.reject(new Error('offline')),
			onDrop
		});
		drag.begin(3);
		await Promise.resolve();
		drag.drop(4);
		expect(onDrop).toHaveBeenCalledWith(3, 4);
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
