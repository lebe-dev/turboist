import type { DayPart } from '$lib/api/types';

export interface DayPartDrag {
	/** The task being carried, or null while no gesture is running. */
	readonly draggedId: number | null;
	/** The phase the carried task was picked up from. */
	readonly origin: DayPart | null;
	/** The phase under the pointer, or null over nothing that takes a drop. */
	readonly hover: DayPart | null;
	/** Where the task would land if let go now — null over its own phase or nothing. */
	readonly landing: DayPart | null;
	begin(taskId: number): void;
	over(part: DayPart | null): void;
	/** Ends the gesture; moves the task when it was let go over another phase. */
	drop(part: DayPart | null): void;
	cancel(): void;
}

/**
 * State of carrying a task between the phases of a day on the Today page.
 *
 * The mouse (HTML5 drag events on TaskItem and the phase sections) and touch
 * (the dnd.ts long-press drag) drive the same calls, so the highlight and the
 * drop bar look the same whichever started the gesture. A task the page does
 * not file under any phase — an overdue one — is never picked up: a new phase
 * would leave it overdue all the same.
 */
export function useDayPartDrag(opts: {
	originOf: (taskId: number) => DayPart | null;
	onMove: (taskId: number, part: DayPart) => void;
}): DayPartDrag {
	let draggedId = $state<number | null>(null);
	let origin = $state<DayPart | null>(null);
	let hover = $state<DayPart | null>(null);

	function reset(): void {
		draggedId = null;
		origin = null;
		hover = null;
	}

	return {
		get draggedId() {
			return draggedId;
		},
		get origin() {
			return origin;
		},
		get hover() {
			return hover;
		},
		get landing() {
			return draggedId !== null && hover !== null && hover !== origin ? hover : null;
		},
		begin(taskId: number) {
			const from = opts.originOf(taskId);
			if (from === null) {
				reset();
				return;
			}
			draggedId = taskId;
			origin = from;
			hover = null;
		},
		over(part: DayPart | null) {
			if (draggedId === null) return;
			hover = part;
		},
		drop(part: DayPart | null) {
			const taskId = draggedId;
			const from = origin;
			reset();
			if (taskId === null || part === null || part === from) return;
			opts.onMove(taskId, part);
		},
		cancel() {
			reset();
		}
	};
}
