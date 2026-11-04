import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { projectsStore } from '$lib/stores/projects.svelte';
import { contextsStore } from '$lib/stores/contexts.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';
import type { Project, ProjectStatsItem, ProjectStatsResponse } from '$lib/api/types';
import ProjectsPage from './+page.svelte';

const statsMock = vi.fn<() => Promise<ProjectStatsResponse>>();

vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({ fetch: vi.fn() })
}));

vi.mock('$lib/api/endpoints/projects', () => ({
	projects: {
		stats: () => statsMock(),
		list: vi.fn()
	}
}));

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

function item(projectId: number, completed: number): ProjectStatsItem {
	const stat = { completed, previousCompleted: 0 };
	return { projectId, periods: { week: stat, month: stat, quarter: stat } };
}

function response(items: ProjectStatsItem[]): ProjectStatsResponse {
	const range = { start: '2026-07-20T00:00:00.000Z', end: '2026-07-27T00:00:00.000Z', days: 7 };
	return { ranges: { week: range, month: range, quarter: range }, items };
}

beforeEach(() => {
	statsMock.mockReset();
	settingsStore.setValue({ ...settingsStore.value, publicView: false });
	contextsStore.clear();
	projectsStore.setItems([
		makeProject(1, 'Alpha'),
		makeProject(2, 'Bravo'),
		makeProject(3, 'Charlie')
	]);
});

afterEach(() => {
	projectsStore.clear();
	settingsStore.clear();
	vi.clearAllMocks();
});

describe('Projects activity radar', () => {
	it('is collapsed by default and does not touch the API', () => {
		render(ProjectsPage);

		const toggle = screen.getByRole('button', { name: /Activity radar/ });
		expect(toggle.getAttribute('aria-expanded')).toBe('false');
		expect(screen.queryByRole('img', { name: /Radar of project activity/ })).toBeNull();
		expect(statsMock).not.toHaveBeenCalled();
	});

	it('loads the report on first expand and draws the radar', async () => {
		statsMock.mockResolvedValue(response([item(1, 4), item(2, 2), item(3, 1)]));
		render(ProjectsPage);

		await fireEvent.click(screen.getByRole('button', { name: /Activity radar/ }));

		expect(await screen.findByRole('img', { name: /Radar of project activity/ })).toBeTruthy();
		expect(statsMock).toHaveBeenCalledTimes(1);
	});

	it('does not refetch when collapsed and expanded again or when the period changes', async () => {
		statsMock.mockResolvedValue(response([item(1, 4), item(2, 2), item(3, 1)]));
		render(ProjectsPage);
		const toggle = screen.getByRole('button', { name: /Activity radar/ });

		await fireEvent.click(toggle);
		await screen.findByRole('img', { name: /Radar of project activity/ });
		await fireEvent.click(screen.getByRole('radio', { name: 'Month' }));
		await fireEvent.click(toggle);
		await fireEvent.click(toggle);

		expect(statsMock).toHaveBeenCalledTimes(1);
	});

	it('says so when too few projects were active to make a shape', async () => {
		statsMock.mockResolvedValue(response([item(1, 4)]));
		render(ProjectsPage);

		await fireEvent.click(screen.getByRole('button', { name: /Activity radar/ }));

		expect(await screen.findByText(/at least 3 projects/)).toBeTruthy();
		expect(screen.queryByRole('img', { name: /Radar of project activity/ })).toBeNull();
	});
});
