import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import type { Task } from '$lib/api/types';
import TaskItem from './TaskItem.svelte';

function task(overrides: Partial<Task> = {}): Task {
	return {
		id: 7,
		title: 'Plant tulips',
		description: '',
		inboxId: null,
		contextId: 1,
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
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		autoSortedAt: null,
		autoSortUndecidedAt: null,
		blockedByCount: 0,
		relationCount: 0,
		labels: [],
		url: '',
		createdAt: '2026-09-13T09:00:00.000Z',
		updatedAt: '2026-09-13T09:00:00.000Z',
		...overrides
	};
}

describe('TaskItem auto-sorted marker', () => {
	it('shows the marker for a task filed by the Inbox processor', () => {
		render(TaskItem, { props: { task: task({ autoSortedAt: '2026-09-13T10:00:00.000Z' }) } });
		const marker = screen.getByTestId('task-auto-sorted');
		expect(marker.getAttribute('title')).toMatch(/AI|ИИ/);
	});

	it('shows no marker for a task placed by a person', () => {
		render(TaskItem, { props: { task: task() } });
		expect(screen.queryByTestId('task-auto-sorted')).toBeNull();
	});

	it('shows the undecided marker for a task the model could not place', () => {
		render(TaskItem, {
			props: {
				task: task({ inboxId: 1, contextId: null, autoSortUndecidedAt: '2026-09-13T10:00:00.000Z' })
			}
		});
		expect(screen.getByTestId('task-auto-sort-undecided')).toBeTruthy();
		expect(screen.queryByTestId('task-auto-sorted')).toBeNull();
	});
});
