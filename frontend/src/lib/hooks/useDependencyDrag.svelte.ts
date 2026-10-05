import type { Task } from '$lib/api/types';
import {
	dependencyCandidates,
	isNestNoop,
	nestRefusal,
	refusalFromServer,
	structuralRefusal,
	type DependencyRefusal,
	type DropMode,
	type NestRefusal
} from '$lib/utils/dependencyDrop';

/** Server-side refusals by candidate task id; absent = acceptable. */
export type DependencyRefusals = Readonly<Record<number, DependencyRefusal>>;

/** Either mode's refusal — the two never overlap, so callers switch on `mode` to know which. */
export type SubtaskDropRefusal = DependencyRefusal | NestRefusal;

export interface DependencyHover {
	targetId: number;
	mode: DropMode;
	/** Pointer position in viewport coordinates, for the floating tooltip. */
	x: number;
	y: number;
}

export interface DependencyDrag {
	/** The subtask being dragged, or null while no gesture is running. */
	readonly draggedId: number | null;
	readonly hover: DependencyHover | null;
	begin(draggedId: number): void;
	over(targetId: number | null, mode: DropMode | null, x: number, y: number): void;
	refusalFor(targetId: number, mode: DropMode): SubtaskDropRefusal | null;
	/** Ends the gesture; commits it when (targetId, mode) is an acceptable drop. */
	drop(targetId: number | null, mode: DropMode | null): void;
	cancel(): void;
}

/**
 * State of the two related gestures on the task page's subtask list: dropping
 * one subtask onto another either makes the dropped one wait for it
 * (`dependency`, edges of the row) or nests the dropped one under it
 * (`nest`, the row's middle band) — see `dropModeForOffset`. Both the mouse
 * (HTML5 drag events in TaskItem) and touch (the dnd.ts long-press drag) drive
 * the same four calls, so the tooltip and the refusals look identical whichever
 * started the gesture.
 *
 * The tree refusals (ancestor/descendant/completed for dependency; descendant/
 * completed for nest) are answered at once from the list. A dependency's
 * duplicate/cycle needs the whole blocking graph, so `begin` asks the server once
 * for every candidate row; until the answer lands a row reads as acceptable for
 * that check, and the server re-checks on write anyway. Nesting has no such
 * server precheck — moving a task never conflicts the way a relation can.
 */
export function useDependencyDrag(opts: {
	tasks: () => readonly Task[];
	check: (draggedId: number, candidateIds: number[]) => Promise<DependencyRefusals>;
	onDependency: (draggedId: number, targetId: number) => void;
	onNest: (draggedId: number, targetId: number) => void;
}): DependencyDrag {
	let draggedId = $state<number | null>(null);
	let hover = $state<DependencyHover | null>(null);
	let serverRefusals = $state<DependencyRefusals>({});
	// Bumped per gesture so a slow check from an earlier drag cannot land on this one.
	let gesture = 0;

	function reset(): void {
		draggedId = null;
		hover = null;
		serverRefusals = {};
	}

	function refusalFor(targetId: number, mode: DropMode): SubtaskDropRefusal | null {
		if (draggedId === null) return null;
		if (mode === 'nest') return nestRefusal(opts.tasks(), draggedId, targetId);
		return structuralRefusal(opts.tasks(), draggedId, targetId) ?? serverRefusals[targetId] ?? null;
	}

	return {
		get draggedId() {
			return draggedId;
		},
		get hover() {
			return hover;
		},
		begin(id: number) {
			gesture += 1;
			const mine = gesture;
			draggedId = id;
			hover = null;
			serverRefusals = {};
			const candidates = dependencyCandidates(opts.tasks(), id);
			if (candidates.length === 0) return;
			opts
				.check(id, candidates)
				.then((refusals) => {
					if (mine === gesture && draggedId === id) serverRefusals = refusals;
				})
				// Offline or refused: keep the tree refusals only — the write itself is
				// still checked by the server and its refusal toasts.
				.catch(() => undefined);
		},
		over(targetId: number | null, mode: DropMode | null, x: number, y: number) {
			if (draggedId === null) return;
			if (targetId === null || mode === null || targetId === draggedId) {
				hover = null;
				return;
			}
			// A nest that would change nothing is not offered as a target, the same
			// way the dragged row itself is not.
			if (mode === 'nest' && isNestNoop(opts.tasks(), draggedId, targetId)) {
				hover = null;
				return;
			}
			hover = { targetId, mode, x, y };
		},
		refusalFor,
		drop(targetId: number | null, mode: DropMode | null) {
			const from = draggedId;
			if (from === null || targetId === null || mode === null || targetId === from) {
				reset();
				return;
			}
			const blocked =
				(mode === 'nest' && isNestNoop(opts.tasks(), from, targetId)) ||
				refusalFor(targetId, mode) !== null;
			reset();
			if (blocked) return;
			if (mode === 'dependency') opts.onDependency(from, targetId);
			else opts.onNest(from, targetId);
		},
		cancel() {
			reset();
		}
	};
}

export function refusalsFromCheck(
	refused: { taskId: number; reason: Parameters<typeof refusalFromServer>[0] }[]
): DependencyRefusals {
	const out: Record<number, DependencyRefusal> = {};
	for (const r of refused) {
		const refusal = refusalFromServer(r.reason);
		if (refusal) out[r.taskId] = refusal;
	}
	return out;
}
