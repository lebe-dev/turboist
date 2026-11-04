import { describe, expect, it, vi } from 'vitest';
import type { DayPart } from '$lib/api/types';
import { useDayPartDrag } from './useDayPartDrag.svelte';

// Task 1 is planned for the morning, task 2 for the evening; task 9 is not on the day at all.
const phases: Record<number, DayPart> = { 1: 'morning', 2: 'evening' };

function setup() {
	const onMove = vi.fn();
	const drag = useDayPartDrag({ originOf: (id) => phases[id] ?? null, onMove });
	return { drag, onMove };
}

describe('useDayPartDrag', () => {
	it('moves a task dropped on another phase and resets', () => {
		const { drag, onMove } = setup();
		drag.begin(1);
		expect(drag.draggedId).toBe(1);
		expect(drag.origin).toBe('morning');
		drag.over('evening');
		expect(drag.landing).toBe('evening');
		drag.drop('evening');
		expect(onMove).toHaveBeenCalledWith(1, 'evening');
		expect(drag.draggedId).toBeNull();
		expect(drag.hover).toBeNull();
	});

	it('does nothing when the task is dropped on the phase it came from', () => {
		const { drag, onMove } = setup();
		drag.begin(1);
		drag.over('morning');
		expect(drag.landing).toBeNull();
		drag.drop('morning');
		expect(onMove).not.toHaveBeenCalled();
		expect(drag.draggedId).toBeNull();
	});

	it('does nothing when the task is let go over nothing', () => {
		const { drag, onMove } = setup();
		drag.begin(2);
		drag.over('morning');
		drag.over(null);
		expect(drag.landing).toBeNull();
		drag.drop(null);
		expect(onMove).not.toHaveBeenCalled();
	});

	it('does not pick up a task that is not in any phase of the day', () => {
		const { drag, onMove } = setup();
		drag.begin(9);
		expect(drag.draggedId).toBeNull();
		drag.over('evening');
		drag.drop('evening');
		expect(onMove).not.toHaveBeenCalled();
	});

	it('ignores hovering and dropping while nothing is carried', () => {
		const { drag, onMove } = setup();
		drag.over('evening');
		expect(drag.hover).toBeNull();
		drag.drop('evening');
		expect(onMove).not.toHaveBeenCalled();
	});

	it('cancel ends the gesture without a move', () => {
		const { drag, onMove } = setup();
		drag.begin(1);
		drag.over('afternoon');
		drag.cancel();
		expect(drag.draggedId).toBeNull();
		expect(drag.landing).toBeNull();
		expect(onMove).not.toHaveBeenCalled();
	});
});
