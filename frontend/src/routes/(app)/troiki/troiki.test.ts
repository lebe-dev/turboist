import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { troikiStore } from '$lib/stores/troiki.svelte';
import { configStore } from '$lib/stores/config.svelte';
import { projectsStore } from '$lib/stores/projects.svelte';
import type { Project, Task, TroikiCategory, TroikiViewResponse } from '$lib/api/types';
import TroikiPage from './+page.svelte';

vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({ fetch: vi.fn() })
}));

vi.mock('$lib/api/endpoints/troiki', () => ({
	troiki: {
		view: vi.fn(async () => troikiStore.value),
		start: vi.fn(async () => troikiStore.value)
	}
}));

vi.mock('$lib/api/endpoints/tasks', () => ({
	tasks: {
		complete: vi.fn(),
		uncomplete: vi.fn()
	}
}));

vi.mock('$lib/api/endpoints/projects', () => ({
	projects: {
		createTask: vi.fn(async (_c: unknown, _id: number, input: Partial<Task>) => ({
			...input,
			id: 999
		}))
	}
}));

function makeProject(over: Partial<Project> = {}): Project {
	return {
		id: 1,
		contextId: 1,
		title: 'Demo',
		description: '',
		color: '#fff',
		status: 'open',
		isPinned: false,
		pinnedAt: null,
		labels: [],
		troikiCategory: null,
		createdAt: '',
		updatedAt: '',
		...over
	};
}

function makeTask(id: number, projectId: number, over: Partial<Task> = {}): Task {
	return {
		id,
		title: `task-${id}`,
		description: '',
		inboxId: null,
		contextId: null,
		projectId,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: '',
		...over
	};
}

function hydrate(view: Partial<TroikiViewResponse>): void {
	const merged: TroikiViewResponse = {
		important: { capacity: 3, projects: [] },
		medium: { capacity: 0, projects: [] },
		rest: { capacity: 0, projects: [] },
		started: false,
		...view
	};
	troikiStore.value = merged;
}

beforeEach(() => {
	troikiStore.clear();
	projectsStore.clear();
	configStore.value = null;
});

afterEach(() => {
	vi.clearAllMocks();
});

describe('Troiki page render', () => {
	it('shows section headers', async () => {
		hydrate({});
		render(TroikiPage);
		expect(await screen.findByRole('heading', { name: 'Important' })).toBeTruthy();
		expect(screen.getByRole('heading', { name: 'Medium' })).toBeTruthy();
		expect(screen.getByRole('heading', { name: 'Rest' })).toBeTruthy();
	});

	it('renders project header and tasks for a Troiki project', async () => {
		const t = makeTask(1, 10, { title: 'first task' });
		hydrate({
			important: {
				capacity: 3,
				projects: [
					{
						...makeProject({ id: 10, title: 'Side hustle', troikiCategory: 'important' }),
						tasks: [t]
					}
				]
			}
		});
		projectsStore.upsert(makeProject({ id: 10, title: 'Side hustle', troikiCategory: 'important' }));
		render(TroikiPage);
		expect(await screen.findByText('Side hustle')).toBeTruthy();
		expect(await screen.findByText('first task')).toBeTruthy();
	});

	it('shows empty-slot placeholders for unfilled capacity', async () => {
		hydrate({ important: { capacity: 3, projects: [] }, started: true });
		render(TroikiPage);
		const empties = await screen.findAllByText(/Empty slot/);
		expect(empties.length).toBeGreaterThanOrEqual(3);
	});

	it('shows initial-mode placeholder before Start when no projects assigned', async () => {
		hydrate({ medium: { capacity: 0, projects: [] }, rest: { capacity: 0, projects: [] } });
		render(TroikiPage);
		const hints = await screen.findAllByText(/Assign to Troiki/);
		expect(hints.length).toBeGreaterThan(0);
	});
});

describe('Troiki page categories', () => {
	const cats: TroikiCategory[] = ['important', 'medium', 'rest'];
	it.each(cats)('hydrates %s slot from store', (cat) => {
		hydrate({ [cat]: { capacity: 1, projects: [] } } as Partial<TroikiViewResponse>);
		expect(troikiStore.value[cat].capacity).toBe(1);
	});
});
