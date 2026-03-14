import type { Task } from '$lib/api/types';

export interface PendingAction {
	parentId: string | null;
	parentContent: string;
	completedTaskLabels: string[];
	completedTaskContent: string;
}

function createNextActionStore() {
	let pendingAction = $state<PendingAction | null>(null);

	function trigger(task: Task, parentContent: string) {
		pendingAction = {
			parentId: task.parent_id ?? null,
			parentContent,
			completedTaskLabels: [...task.labels],
			completedTaskContent: task.content
		};
	}

	/** Trigger for a standalone task (no parent, no subtasks) — follow-up task */
	function triggerFollowUp(task: Task) {
		pendingAction = {
			parentId: null,
			parentContent: task.content,
			completedTaskLabels: [...task.labels],
			completedTaskContent: task.content
		};
	}

	function dismiss() {
		pendingAction = null;
	}

	function consume(): PendingAction | null {
		const action = pendingAction;
		pendingAction = null;
		return action;
	}

	return {
		get pendingAction() {
			return pendingAction;
		},
		trigger,
		triggerFollowUp,
		dismiss,
		consume
	};
}

export const nextActionStore = createNextActionStore();
