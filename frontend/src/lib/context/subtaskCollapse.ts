export const SUBTASK_COLLAPSE_KEY = 'turboist:subtaskCollapse';

export interface SubtaskCollapseCtx {
	readonly ids: Set<number>;
	/** True when every parent task's subtasks are collapsed ("collapse all" active). */
	readonly allCollapsed: boolean;
	toggle(id: number): void;
}
