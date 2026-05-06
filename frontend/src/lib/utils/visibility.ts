import type { Label, Project, Task } from '../api/types';

export function isProjectVisible(project: Project, publicView: boolean): boolean {
	if (!publicView) return true;
	return !project.isPrivate;
}

export function isLabelVisible(label: Label, publicView: boolean): boolean {
	if (!publicView) return true;
	return !label.isPrivate;
}

export function isTaskVisible(
	task: Task,
	publicView: boolean,
	projectsById: Map<number, Project>,
	tasksById: Map<number, Task>
): boolean {
	if (!publicView) return true;
	if (task.isPrivate) return false;
	if (task.projectId !== null) {
		const project = projectsById.get(task.projectId);
		if (project && project.isPrivate) return false;
	}
	const seen = new Set<number>();
	let parentId = task.parentId;
	while (parentId !== null && !seen.has(parentId)) {
		seen.add(parentId);
		const parent = tasksById.get(parentId);
		if (!parent) break;
		if (parent.isPrivate) return false;
		parentId = parent.parentId;
	}
	return true;
}

export function buildTasksById(tasks: Task[]): Map<number, Task> {
	const map = new Map<number, Task>();
	for (const t of tasks) map.set(t.id, t);
	return map;
}

export function buildProjectsById(projects: Project[]): Map<number, Project> {
	const map = new Map<number, Project>();
	for (const p of projects) map.set(p.id, p);
	return map;
}
