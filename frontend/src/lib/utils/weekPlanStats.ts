import type { Label, Project, Task } from '../api/types';

export interface LabelUsage {
	label: Label;
	count: number;
}

export interface WeekPlanStats {
	// Labels used by the open planned tasks, most used first; ties by name.
	labels: LabelUsage[];
	// Open planned tasks whose project stands in a Troiki bucket.
	fromTroiki: number;
	// Open planned tasks the counters were taken over.
	total: number;
}

// weekPlanStats summarises what the week being planned is made of. Subtasks count
// on their own: planning a parent cascades onto them, so they are planned work too.
export function weekPlanStats(tasks: Task[], projects: Project[]): WeekPlanStats {
	const troikiProjects = new Set(
		projects.filter((p) => p.troikiCategory !== null).map((p) => p.id)
	);
	const usage = new Map<number, LabelUsage>();
	let fromTroiki = 0;
	let total = 0;
	for (const task of tasks) {
		if (task.status !== 'open') continue;
		total++;
		if (task.projectId !== null && troikiProjects.has(task.projectId)) fromTroiki++;
		for (const label of task.labels) {
			const entry = usage.get(label.id);
			if (entry) entry.count++;
			else usage.set(label.id, { label, count: 1 });
		}
	}
	const labels = [...usage.values()].sort(
		(a, b) => b.count - a.count || a.label.name.localeCompare(b.label.name)
	);
	return { labels, fromTroiki, total };
}
