import { describe, it, expect } from 'vitest';
import {
	buildProjectsById,
	buildTasksById,
	isLabelVisible,
	isProjectVisible,
	isTaskVisible
} from './visibility';
import type { Label, Project, Task } from '../api/types';

function project(id: number, isPrivate = false): Project {
	return {
		id,
		contextId: 1,
		title: `p${id}`,
		description: '',
		color: '#fff',
		status: 'open',
		isPinned: false,
		pinnedAt: null,
		isPrivate,
		labels: [],
		troikiCategory: null,
		createdAt: '',
		updatedAt: ''
	};
}

function task(id: number, over: Partial<Task> = {}): Task {
	return {
		id,
		title: `t${id}`,
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
		isPrivate: false,
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

function label(id: number, isPrivate = false): Label {
	return {
		id,
		name: `l${id}`,
		color: '#fff',
		isFavourite: false,
		isPrivate,
		createdAt: '',
		updatedAt: ''
	};
}

describe('isProjectVisible', () => {
	it('returns true for any project when publicView is off', () => {
		expect(isProjectVisible(project(1, true), false)).toBe(true);
	});
	it('hides private project when publicView is on', () => {
		expect(isProjectVisible(project(1, true), true)).toBe(false);
	});
	it('shows non-private project when publicView is on', () => {
		expect(isProjectVisible(project(1, false), true)).toBe(true);
	});
});

describe('isLabelVisible', () => {
	it('hides private label only when publicView is on', () => {
		expect(isLabelVisible(label(1, true), false)).toBe(true);
		expect(isLabelVisible(label(1, true), true)).toBe(false);
		expect(isLabelVisible(label(1, false), true)).toBe(true);
	});
});

describe('isTaskVisible', () => {
	it('shows everything when publicView is off', () => {
		const t = task(1, { isPrivate: true });
		expect(isTaskVisible(t, false, new Map(), new Map())).toBe(true);
	});

	it('hides task with own isPrivate flag', () => {
		const t = task(1, { isPrivate: true });
		expect(isTaskVisible(t, true, new Map(), buildTasksById([t]))).toBe(false);
	});

	it('cascades from private project', () => {
		const p = project(10, true);
		const t = task(1, { projectId: 10 });
		expect(isTaskVisible(t, true, buildProjectsById([p]), buildTasksById([t]))).toBe(false);
	});

	it('cascades from private parent task', () => {
		const root = task(1, { isPrivate: true });
		const sub = task(2, { parentId: 1 });
		const subsub = task(3, { parentId: 2 });
		const tasksById = buildTasksById([root, sub, subsub]);
		expect(isTaskVisible(sub, true, new Map(), tasksById)).toBe(false);
		expect(isTaskVisible(subsub, true, new Map(), tasksById)).toBe(false);
	});

	it('keeps task visible when project and ancestors are public', () => {
		const p = project(10, false);
		const root = task(1);
		const sub = task(2, { parentId: 1, projectId: 10 });
		const tasksById = buildTasksById([root, sub]);
		expect(
			isTaskVisible(sub, true, buildProjectsById([p]), tasksById)
		).toBe(true);
	});

	it('handles missing parent gracefully', () => {
		const t = task(2, { parentId: 99 });
		expect(isTaskVisible(t, true, new Map(), new Map())).toBe(true);
	});
});
