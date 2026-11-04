import { describe, it, expect } from 'vitest';
import { buildProjectRadar } from './projectStats';
import type { LabelStatsPeriod, Project, ProjectStatsItem } from '../api/types';

function makeProject(id: number, title: string): Project {
	return {
		id,
		contextId: 1,
		title,
		description: '',
		color: 'blue',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory: null,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z'
	};
}

function makeItem(
	projectId: number,
	week: [number, number],
	month: [number, number] = week
): ProjectStatsItem {
	const stat = ([completed, previousCompleted]: [number, number]) => ({
		completed,
		previousCompleted
	});
	return {
		projectId,
		periods: { week: stat(week), month: stat(month), quarter: stat(month) }
	};
}

describe('buildProjectRadar', () => {
	const projects = [
		makeProject(1, 'Delta'),
		makeProject(2, 'Alpha'),
		makeProject(3, 'Charlie'),
		makeProject(4, 'Bravo'),
		makeProject(5, 'Idle')
	];

	it('returns null below three spokes', () => {
		const items = [makeItem(1, [2, 0]), makeItem(2, [1, 0])];
		expect(buildProjectRadar(projects, items, 'week')).toBeNull();
	});

	it('places spokes by title and scales against the busiest project', () => {
		const items = [makeItem(1, [4, 1]), makeItem(2, [2, 0]), makeItem(3, [1, 8])];
		const radar = buildProjectRadar(projects, items, 'week')!;
		expect(radar.axes.map((a) => a.project.title)).toEqual(['Alpha', 'Charlie', 'Delta']);
		// The previous window counts for the scale: Charlie's 8 is the outer ring.
		expect(radar.scaleMax).toBe(8);
		expect(radar.axes[2].current).toBeCloseTo(0.5);
		expect(radar.axes[1].previous).toBe(1);
		expect(radar.axes[0].angle).toBe(0);
	});

	it('keeps a project on a spoke when it only worked in the previous window', () => {
		const items = [makeItem(1, [0, 3]), makeItem(2, [1, 0]), makeItem(3, [1, 0])];
		const radar = buildProjectRadar(projects, items, 'week')!;
		expect(radar.axes.map((a) => a.project.id)).toContain(1);
		expect(radar.axes.find((a) => a.project.id === 1)!.current).toBe(0);
	});

	it('ignores projects the caller did not pass in (hidden, filtered)', () => {
		const items = [
			makeItem(1, [4, 0]),
			makeItem(2, [2, 0]),
			makeItem(3, [1, 0]),
			makeItem(99, [50, 0])
		];
		const radar = buildProjectRadar(projects, items, 'week')!;
		expect(radar.axes.map((a) => a.project.id)).not.toContain(99);
		expect(radar.scaleMax).toBe(4);
	});

	it('caps spokes at the busiest projects and reports the rest as hidden', () => {
		const items = [
			makeItem(1, [9, 0]),
			makeItem(2, [5, 0]),
			makeItem(3, [3, 0]),
			makeItem(4, [1, 0])
		];
		const radar = buildProjectRadar(projects, items, 'week', 3)!;
		expect(radar.axes.map((a) => a.project.title)).toEqual(['Alpha', 'Charlie', 'Delta']);
		expect(radar.hidden).toBe(1);
	});

	it('reads the selected period', () => {
		const items = [
			makeItem(1, [1, 0], [10, 0]),
			makeItem(2, [1, 0], [5, 0]),
			makeItem(3, [1, 0], [2, 0])
		];
		const period: LabelStatsPeriod = 'month';
		expect(buildProjectRadar(projects, items, period)!.scaleMax).toBe(10);
		expect(buildProjectRadar(projects, items, 'week')!.scaleMax).toBe(1);
	});
});
