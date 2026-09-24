import type { Task } from '$lib/api/types';
import {
	dependencyCandidates,
	refusalFromServer,
	structuralRefusal,
	type DependencyRefusal
} from '$lib/utils/dependencyDrop';

/** Server-side refusals by candidate task id; absent = acceptable. */
export type DependencyRefusals = Readonly<Record<number, DependencyRefusal>>;

export interface DependencyHover {
	targetId: number;
	/** Pointer position in viewport coordinates, for the floating tooltip. */
	x: number;
	y: number;
}

export interface DependencyDrag {
	/** The subtask being dragged, or null while no gesture is running. */
	readonly draggedId: number | null;
	readonly hover: DependencyHover | null;
	begin(draggedId: number): void;
	over(targetId: number | null, x: number, y: number): void;
	refusalFor(targetId: number): DependencyRefusal | null;
	/** Ends the gesture; commits it when `targetId` is an acceptable target. */
	drop(targetId: number | null): void;
	cancel(): void;
}

/**
 * State of the "drop a subtask onto another to make it wait for it" gesture on the
 * task page. Both the mouse (HTML5 drag events in TaskItem) and touch (the dnd.ts
 * long-press drag) drive the same four calls, so the tooltip and the refusals look
 * identical whichever started the gesture.
 *
 * The tree refusals (ancestor/descendant/completed) are answered at once from the
 * list. Duplicates and cycles need the whole blocking graph, so `begin` asks the
 * server once for every candidate row; until the answer lands a row reads as
 * acceptable, and the server re-checks on write anyway.
 */
export function useDependencyDrag(opts: {
	tasks: () => readonly Task[];
	check: (draggedId: number, candidateIds: number[]) => Promise<DependencyRefusals>;
	onDrop: (draggedId: number, targetId: number) => void;
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

	function refusalFor(targetId: number): DependencyRefusal | null {
		if (draggedId === null) return null;
		return (
			structuralRefusal(opts.tasks(), draggedId, targetId) ?? serverRefusals[targetId] ?? null
		);
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
		over(targetId: number | null, x: number, y: number) {
			if (draggedId === null) return;
			if (targetId === null || targetId === draggedId) {
				hover = null;
				return;
			}
			hover = { targetId, x, y };
		},
		refusalFor,
		drop(targetId: number | null) {
			const from = draggedId;
			const accepted =
				from !== null && targetId !== null && targetId !== from && refusalFor(targetId) === null;
			reset();
			if (accepted) opts.onDrop(from, targetId);
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
