import type { LabelStatsPeriod, Project, ProjectStatsItem } from '../api/types';
import { LABEL_RADAR_MAX_AXES, LABEL_RADAR_MIN_AXES } from './labelStats';

/**
 * One spoke of the project radar. `current` and `previous` are fractions of the
 * chart's radius (0..1) for the selected window and the one before it; `angle`
 * is in radians, clockwise from twelve o'clock.
 */
export interface ProjectRadarAxis {
	project: Project;
	completed: number;
	previousCompleted: number;
	angle: number;
	current: number;
	previous: number;
}

export interface ProjectRadar {
	axes: ProjectRadarAxis[];
	/** The completion count the outer ring stands for. */
	scaleMax: number;
	/** Projects with activity that did not get a spoke because of the cap. */
	hidden: number;
}

/**
 * Lay the projects out as a radar, the same way the label radar is: every
 * project is a direction, the shape shows where the finished work went in the
 * selected window, and the window before it is a second outline to compare
 * against.
 *
 * A project earns a spoke when it completed something in either window. Past the
 * cap the busiest projects win; spokes are then placed by title so a project
 * keeps its direction when the period changes. Returns null when there are too
 * few spokes to make a shape.
 */
export function buildProjectRadar(
	projects: Project[],
	items: ProjectStatsItem[],
	period: LabelStatsPeriod,
	maxAxes: number = LABEL_RADAR_MAX_AXES
): ProjectRadar | null {
	const byId = new Map(items.map((item) => [item.projectId, item.periods[period]]));
	const rows = projects.flatMap((project) => {
		const stats = byId.get(project.id);
		if (!stats) return [];
		return [{ project, completed: stats.completed, previousCompleted: stats.previousCompleted }];
	});
	const weight = (row: { completed: number; previousCompleted: number }) =>
		Math.max(row.completed, row.previousCompleted);
	const candidates = rows
		.filter((row) => weight(row) > 0)
		.sort((a, b) => weight(b) - weight(a) || a.project.title.localeCompare(b.project.title));
	const picked = candidates
		.slice(0, maxAxes)
		.sort((a, b) => a.project.title.localeCompare(b.project.title) || a.project.id - b.project.id);
	if (picked.length < LABEL_RADAR_MIN_AXES) return null;
	const scaleMax = Math.max(1, ...picked.map(weight));
	return {
		axes: picked.map((row, index) => ({
			...row,
			angle: (2 * Math.PI * index) / picked.length,
			current: row.completed / scaleMax,
			previous: row.previousCompleted / scaleMax
		})),
		scaleMax,
		hidden: candidates.length - picked.length
	};
}
