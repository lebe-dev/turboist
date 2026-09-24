import type { BlockerRefusalReason, Task } from '../api/types';

/**
 * Why a dragged subtask cannot be made to depend on the row under the pointer.
 * `ancestor`/`descendant` are refused on the device: blockers are inherited down
 * the subtask tree, so a dependency along one branch would either block a task on
 * its own subtree or be meaningless. `exists`/`cycle` come from the server's
 * blocker check, which alone sees the whole blocking graph.
 */
export type DependencyRefusal = 'ancestor' | 'descendant' | 'completed' | 'exists' | 'cycle';

function ancestorIds(byId: Map<number, Task>, id: number): Set<number> {
	const out = new Set<number>();
	let parentId = byId.get(id)?.parentId ?? null;
	while (parentId !== null && !out.has(parentId)) {
		out.add(parentId);
		parentId = byId.get(parentId)?.parentId ?? null;
	}
	return out;
}

/**
 * The refusal that follows from the subtask tree alone, or null when the pair is
 * fine as far as the list can tell. `draggedId === targetId` is not a target at
 * all and answers null; callers never offer a row to itself.
 */
export function structuralRefusal(
	tasks: readonly Task[],
	draggedId: number,
	targetId: number
): DependencyRefusal | null {
	if (draggedId === targetId) return null;
	const byId = new Map(tasks.map((t) => [t.id, t] as const));
	const target = byId.get(targetId);
	if (!target) return null;
	if (target.status !== 'open') return 'completed';
	if (ancestorIds(byId, draggedId).has(targetId)) return 'ancestor';
	if (ancestorIds(byId, targetId).has(draggedId)) return 'descendant';
	return null;
}

/** Rows worth asking the server about: every other open task the tree allows. */
export function dependencyCandidates(tasks: readonly Task[], draggedId: number): number[] {
	return tasks
		.filter((t) => t.id !== draggedId && t.id > 0)
		.filter((t) => structuralRefusal(tasks, draggedId, t.id) === null)
		.map((t) => t.id);
}

/** Maps a blocker-check reason onto what the gesture shows; `null` = not shown. */
export function refusalFromServer(reason: BlockerRefusalReason): DependencyRefusal | null {
	if (reason === 'relation_exists') return 'exists';
	if (reason === 'relation_cycle') return 'cycle';
	// relation_self / not_found cannot reach a visible row: the row is the dragged
	// one (never offered) or already gone (the next refetch removes it).
	return null;
}

/**
 * Which of the two things a drop over a row means, given where inside its
 * height the pointer sits. The edges read as "relate to this row" (dependency,
 * marked by a padlock); the middle band reads as "go inside this row" (nest,
 * marked by an indent arrow) — the same split a file manager's tree uses to tell
 * "drop next to" from "drop into".
 */
export type DropMode = 'dependency' | 'nest';

// The top/bottom fraction of a row's height that means "dependency" rather than
// "nest". A quarter on each edge leaves the wide middle band for the nest that
// most drags are aiming for, while keeping edges easy to land on deliberately.
const DEPENDENCY_EDGE_FRACTION = 0.25;

export function dropModeForOffset(relativeY: number): DropMode {
	return relativeY < DEPENDENCY_EDGE_FRACTION || relativeY > 1 - DEPENDENCY_EDGE_FRACTION
		? 'dependency'
		: 'nest';
}

/** Why a dragged subtask cannot be nested under the row under the pointer. */
export type NestRefusal = 'descendant' | 'completed';

/**
 * Whether nesting draggedId under targetId — making it targetId's subtask —
 * would change nothing: targetId is already its parent. Not a refusal, just
 * nothing to offer, the same way the dragged row itself is not offered.
 */
export function isNestNoop(tasks: readonly Task[], draggedId: number, targetId: number): boolean {
	return tasks.find((t) => t.id === draggedId)?.parentId === targetId;
}

/**
 * The refusal that follows from the subtask tree for nesting draggedId under
 * targetId, or null when it may go ahead (including the no-op case — see
 * isNestNoop, checked separately since it is not really a refusal).
 */
export function nestRefusal(
	tasks: readonly Task[],
	draggedId: number,
	targetId: number
): NestRefusal | null {
	const byId = new Map(tasks.map((t) => [t.id, t] as const));
	const target = byId.get(targetId);
	if (!target) return null;
	if (target.status !== 'open') return 'completed';
	// targetId sitting inside dragged's own subtree would nest dragged under its
	// own descendant — the one cycle the tree can never resolve. Nesting dragged
	// under one of its own ancestors is fine: that is just moving it up a level.
	if (ancestorIds(byId, targetId).has(draggedId)) return 'descendant';
	return null;
}
