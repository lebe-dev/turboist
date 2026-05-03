import { describe, expect, it, beforeEach } from 'vitest';
import { contextsStore } from './contexts.svelte';
import { projectsStore } from './projects.svelte';
import { labelsStore } from './labels.svelte';
import { troikiStore } from './troiki.svelte';
import type { Context, Label, Project, Task, TroikiCategory } from '$lib/api/types';
import type { TroikiViewResponse } from '$lib/api/types';

function makeContext(over: Partial<Context> = {}): Context {
	return {
		id: 1,
		name: 'Personal',
		color: '#fff',
		isFavourite: false,
		createdAt: '',
		updatedAt: '',
		...over
	};
}
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
function makeTask(
	id: number,
	projectId: number | null = null,
	over: Partial<Task> = {}
): Task {
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

function makeLabel(over: Partial<Label> = {}): Label {
	return {
		id: 1,
		name: 'work',
		color: '#fff',
		isFavourite: false,
		createdAt: '',
		updatedAt: '',
		...over
	};
}

beforeEach(() => {
	contextsStore.clear();
	projectsStore.clear();
	labelsStore.clear();
	troikiStore.clear();
});

describe('contextsStore', () => {
	it('upsert adds and updates', () => {
		contextsStore.upsert(makeContext({ id: 1, name: 'A' }));
		contextsStore.upsert(makeContext({ id: 2, name: 'B' }));
		expect(contextsStore.items.length).toBe(2);
		contextsStore.upsert(makeContext({ id: 1, name: 'A2' }));
		expect(contextsStore.items.find((c) => c.id === 1)?.name).toBe('A2');
	});

	it('remove drops by id', () => {
		contextsStore.upsert(makeContext({ id: 1 }));
		contextsStore.upsert(makeContext({ id: 2 }));
		contextsStore.remove(1);
		expect(contextsStore.items.map((c) => c.id)).toEqual([2]);
	});
});

describe('projectsStore', () => {
	it('byContext filters and pinned derives', () => {
		projectsStore.upsert(makeProject({ id: 1, contextId: 1, isPinned: true }));
		projectsStore.upsert(makeProject({ id: 2, contextId: 2 }));
		expect(projectsStore.byContext(1).map((p) => p.id)).toEqual([1]);
		expect(projectsStore.pinned.map((p) => p.id)).toEqual([1]);
	});
});

describe('labelsStore', () => {
	it('separates favourites and rest', () => {
		labelsStore.upsert(makeLabel({ id: 1, isFavourite: true }));
		labelsStore.upsert(makeLabel({ id: 2, isFavourite: false }));
		expect(labelsStore.favourites.map((l) => l.id)).toEqual([1]);
		expect(labelsStore.rest.map((l) => l.id)).toEqual([2]);
	});
});

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

function makeTroikiProject(
	id: number,
	category: TroikiCategory,
	tasks: Task[] = []
): Project & { tasks: Task[] } {
	return { ...makeProject({ id, troikiCategory: category }), tasks };
}

describe('troikiStore', () => {
	it('clear resets to empty default state', () => {
		hydrate({ important: { capacity: 3, projects: [makeTroikiProject(10, 'important')] } });
		troikiStore.clear();
		expect(troikiStore.value.important.projects).toEqual([]);
		expect(troikiStore.value.medium.projects).toEqual([]);
		expect(troikiStore.value.rest.projects).toEqual([]);
		expect(troikiStore.value.important.capacity).toBe(3);
	});

	it('applyTaskUpdate replaces a task within its owning project', () => {
		const original = makeTask(1, 10, { title: 'old' });
		hydrate({
			important: { capacity: 3, projects: [makeTroikiProject(10, 'important', [original])] }
		});
		const updated = makeTask(1, 10, { title: 'new' });
		troikiStore.applyTaskUpdate(updated);
		expect(troikiStore.value.important.projects[0].tasks).toEqual([updated]);
	});

	it('applyTaskUpdate moves a task between projects across slots', () => {
		const t = makeTask(1, 10);
		hydrate({
			important: { capacity: 3, projects: [makeTroikiProject(10, 'important', [t])] },
			medium: { capacity: 1, projects: [makeTroikiProject(20, 'medium')] }
		});
		troikiStore.applyTaskUpdate(makeTask(1, 20));
		expect(troikiStore.value.important.projects[0].tasks).toEqual([]);
		expect(troikiStore.value.medium.projects[0].tasks.map((x) => x.id)).toEqual([1]);
	});

	it('applyTaskUpdate drops the task when status is no longer open', () => {
		const open = makeTask(1, 10);
		hydrate({
			important: { capacity: 3, projects: [makeTroikiProject(10, 'important', [open])] }
		});
		troikiStore.applyTaskUpdate(makeTask(1, 10, { status: 'completed' }));
		expect(troikiStore.value.important.projects[0].tasks).toEqual([]);
	});

	it('applyTaskUpdate ignores tasks whose project is not in any slot', () => {
		hydrate({
			important: { capacity: 3, projects: [makeTroikiProject(10, 'important')] }
		});
		troikiStore.applyTaskUpdate(makeTask(99, 999));
		expect(troikiStore.value.important.projects[0].tasks).toEqual([]);
	});

	it('applyProjectUpdate moves a project to its new category and preserves its tasks', () => {
		const t = makeTask(1, 10);
		hydrate({
			important: { capacity: 3, projects: [makeTroikiProject(10, 'important', [t])] }
		});
		troikiStore.applyProjectUpdate(makeProject({ id: 10, troikiCategory: 'rest' }));
		expect(troikiStore.value.important.projects).toEqual([]);
		expect(troikiStore.value.rest.projects[0].id).toBe(10);
		expect(troikiStore.value.rest.projects[0].tasks).toEqual([t]);
	});

	it('applyProjectUpdate drops the project when its category is cleared', () => {
		hydrate({
			medium: { capacity: 1, projects: [makeTroikiProject(20, 'medium')] }
		});
		troikiStore.applyProjectUpdate(makeProject({ id: 20, troikiCategory: null }));
		expect(troikiStore.value.medium.projects).toEqual([]);
	});

	it('applyProjectUpdate adds a new project to a slot when it gains a category', () => {
		hydrate({});
		troikiStore.applyProjectUpdate(makeProject({ id: 30, troikiCategory: 'important' }));
		expect(troikiStore.value.important.projects.map((p) => p.id)).toEqual([30]);
		expect(troikiStore.value.important.projects[0].tasks).toEqual([]);
	});

	it('removeTask drops the task from every project across slots', () => {
		hydrate({
			important: {
				capacity: 3,
				projects: [makeTroikiProject(10, 'important', [makeTask(1, 10), makeTask(2, 10)])]
			},
			medium: { capacity: 1, projects: [makeTroikiProject(20, 'medium', [makeTask(3, 20)])] }
		});
		troikiStore.removeTask(1);
		expect(troikiStore.value.important.projects[0].tasks.map((t) => t.id)).toEqual([2]);
		expect(troikiStore.value.medium.projects[0].tasks.map((t) => t.id)).toEqual([3]);
	});
});
