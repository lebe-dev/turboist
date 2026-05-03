import { describe, expect, it, beforeEach } from 'vitest';
import { contextsStore } from './contexts.svelte';
import { projectsStore } from './projects.svelte';
import { labelsStore } from './labels.svelte';
import { troikiStore } from './troiki.svelte';
import type { Context, Label, Project, Task, TroikiCategory } from '$lib/api/types';

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
		createdAt: '',
		updatedAt: '',
		...over
	};
}
function makeTask(
	id: number,
	category: TroikiCategory | null,
	over: Partial<Task> = {}
): Task {
	return {
		id,
		title: `task-${id}`,
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
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
		troikiCategory: category,
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

describe('troikiStore', () => {
	it('clear resets to empty default state', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		troikiStore.clear();
		expect(troikiStore.value.important.tasks).toEqual([]);
		expect(troikiStore.value.medium.tasks).toEqual([]);
		expect(troikiStore.value.rest.tasks).toEqual([]);
		expect(troikiStore.value.important.capacity).toBe(3);
	});

	it('applyTaskUpdate places task in matching category slot', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		expect(troikiStore.value.important.tasks.map((t) => t.id)).toEqual([1]);
		expect(troikiStore.value.medium.tasks).toEqual([]);
	});

	it('applyTaskUpdate moves task between slots when category changes', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		troikiStore.applyTaskUpdate(makeTask(1, 'medium'));
		expect(troikiStore.value.important.tasks).toEqual([]);
		expect(troikiStore.value.medium.tasks.map((t) => t.id)).toEqual([1]);
	});

	it('applyTaskUpdate removes task from all slots when category cleared', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		troikiStore.applyTaskUpdate(makeTask(1, null));
		expect(troikiStore.value.important.tasks).toEqual([]);
	});

	it('applyTaskUpdate removes task from slot when status is no longer open', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		troikiStore.applyTaskUpdate(makeTask(1, 'important', { status: 'completed' }));
		expect(troikiStore.value.important.tasks).toEqual([]);
	});

	it('removeTask drops task from all slots', () => {
		troikiStore.applyTaskUpdate(makeTask(1, 'important'));
		troikiStore.applyTaskUpdate(makeTask(2, 'medium'));
		troikiStore.removeTask(1);
		expect(troikiStore.value.important.tasks).toEqual([]);
		expect(troikiStore.value.medium.tasks.map((t) => t.id)).toEqual([2]);
	});
});
