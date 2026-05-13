export const SUBTASK_COLLAPSE_KEY = 'turboist:subtaskCollapse';

export interface SubtaskCollapseCtx {
	readonly ids: Set<number>;
	toggle(id: number): void;
}
