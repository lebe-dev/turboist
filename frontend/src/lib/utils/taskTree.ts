import type { Task } from '../api/types';

export interface TaskNode {
	task: Task;
	children: TaskNode[];
}

/**
 * Build a tree of tasks from a flat list using `parentId` links.
 * Sibling order is preserved as encountered in the input. Tasks whose
 * `parentId` does not resolve in the input are treated as roots so that
 * partial fetches still render coherently.
 */
export function buildTree(tasks: Task[]): TaskNode[] {
	const byId = new Map<number, TaskNode>();
	for (const task of tasks) {
		byId.set(task.id, { task, children: [] });
	}

	const roots: TaskNode[] = [];
	for (const task of tasks) {
		const node = byId.get(task.id)!;
		if (task.parentId !== null && byId.has(task.parentId)) {
			byId.get(task.parentId)!.children.push(node);
		} else {
			roots.push(node);
		}
	}

	return roots;
}

/**
 * Split a flat task list into two buckets based on the completion status of
 * each task's top-most ancestor in the same list. Descendants of a completed
 * root go to `done`; everything else stays in `open` (so completed children
 * under an open parent are still rendered inline by their parent's tree).
 */
export function splitByRootCompletion(items: Task[]): { open: Task[]; done: Task[] } {
	const byId = new Map(items.map((t) => [t.id, t] as const));
	const open: Task[] = [];
	const done: Task[] = [];
	for (const task of items) {
		let root = task;
		while (root.parentId !== null && byId.has(root.parentId)) {
			root = byId.get(root.parentId)!;
		}
		if (root.status === 'completed') done.push(task);
		else open.push(task);
	}
	return { open, done };
}

export function flattenTree(nodes: TaskNode[]): Task[] {
	const out: Task[] = [];
	const walk = (list: TaskNode[]) => {
		for (const n of list) {
			out.push(n.task);
			if (n.children.length) walk(n.children);
		}
	};
	walk(nodes);
	return out;
}
