import type { Task, Due } from '$lib/api/types';

/**
 * Flat representation of a Task for SyncroState storage.
 * No recursive `children` — tree is derived at read time via buildTree().
 * `due` object flattened into due_date + due_recurring primitives.
 */
export interface FlatTask {
	id: string;
	content: string;
	description: string;
	project_id: string;
	section_id: string | null;
	parent_id: string | null;
	labels: string[];
	priority: number;
	due_date: string | null;
	due_recurring: boolean;
	sub_task_count: number;
	completed_sub_task_count: number;
	completed_at: string | null;
	added_at: string;
	is_project_task: boolean;
	postpone_count: number;
}

export function taskToFlat(task: Task): FlatTask {
	return {
		id: task.id,
		content: task.content,
		description: task.description,
		project_id: task.project_id,
		section_id: task.section_id,
		parent_id: task.parent_id,
		labels: [...task.labels],
		priority: task.priority,
		due_date: task.due?.date ?? null,
		due_recurring: task.due?.recurring ?? false,
		sub_task_count: task.sub_task_count,
		completed_sub_task_count: task.completed_sub_task_count,
		completed_at: task.completed_at,
		added_at: task.added_at,
		is_project_task: task.is_project_task,
		postpone_count: task.postpone_count ?? 0
	};
}

export function flatToTask(flat: FlatTask, children: Task[] = []): Task {
	const due: Due | null = flat.due_date ? { date: flat.due_date, recurring: flat.due_recurring } : null;
	return {
		id: flat.id,
		content: flat.content,
		description: flat.description,
		project_id: flat.project_id,
		section_id: flat.section_id,
		parent_id: flat.parent_id,
		labels: [...flat.labels],
		priority: flat.priority,
		due: due,
		sub_task_count: flat.sub_task_count,
		completed_sub_task_count: flat.completed_sub_task_count,
		completed_at: flat.completed_at,
		added_at: flat.added_at,
		is_project_task: flat.is_project_task,
		postpone_count: flat.postpone_count,
		children
	};
}

/** Depth-first flatten: tree Task[] → FlatTask[] preserving sibling order. */
export function flattenTasks(tasks: Task[]): FlatTask[] {
	const result: FlatTask[] = [];
	function walk(list: Task[]) {
		for (const task of list) {
			result.push(taskToFlat(task));
			if (task.children.length > 0) walk(task.children);
		}
	}
	walk(tasks);
	return result;
}

/** Rebuild tree from flat array using parent_id references. Orphans become roots. */
export function buildTree(flats: FlatTask[]): Task[] {
	const byId = new Map<string, Task>();
	const roots: Task[] = [];

	for (const f of flats) {
		byId.set(f.id, flatToTask(f));
	}

	for (const f of flats) {
		const node = byId.get(f.id)!;
		if (f.parent_id && byId.has(f.parent_id)) {
			byId.get(f.parent_id)!.children.push(node);
		} else {
			roots.push(node);
		}
	}

	return roots;
}
