import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/errors';
import type { Task } from '$lib/api/types';
import { describeError, isBlocked, taskToJson } from './taskActions';

describe('describeError', () => {
	it('returns the ApiError message when present', () => {
		const err = new ApiError('validation_failed', 'Title is required', 422);
		expect(describeError(err, 'Fallback')).toBe('Title is required');
	});

	it('falls back when the ApiError has an empty message', () => {
		const err = new ApiError('internal_error', '', 500);
		expect(describeError(err, 'Fallback')).toBe('Fallback');
	});

	it('returns the message of a generic Error', () => {
		expect(describeError(new Error('boom'), 'Fallback')).toBe('boom');
	});

	it('returns the fallback for non-Error values', () => {
		expect(describeError('string thrown', 'Fallback')).toBe('Fallback');
		expect(describeError(undefined, 'Fallback')).toBe('Fallback');
		expect(describeError(null, 'Fallback')).toBe('Fallback');
		expect(describeError({ code: 'x' }, 'Fallback')).toBe('Fallback');
	});

	it('maps the offline_unsupported code to the localized message, ignoring the raw text', () => {
		const err = new ApiError('offline_unsupported', 'action unavailable offline', 0);
		expect(describeError(err, 'Fallback')).toBe('Unavailable offline');
	});

	it('maps a status-0 network error / timeout to the localized needs-connection message', () => {
		// WebKit surfaces a raw "Load failed"; the offline layer already tried (and
		// missed) the cache, so the user should see a clear message, not that text.
		expect(describeError(new ApiError('network_error', 'Load failed', 0), 'Fallback')).toBe(
			'No connection — this page hasn\'t been opened online yet, so there\'s nothing cached to show.'
		);
		expect(describeError(new ApiError('timeout', 'request timed out', 0), 'Fallback')).toBe(
			'No connection — this page hasn\'t been opened online yet, so there\'s nothing cached to show.'
		);
	});

	it('does not treat a real HTTP error carrying status 0-like codes as offline', () => {
		// A 5xx with a message must still surface its own message, not the offline one.
		const err = new ApiError('internal_error', 'server exploded', 500);
		expect(describeError(err, 'Fallback')).toBe('server exploded');
	});
});

describe('describeError task_blocked', () => {
	// The client normally refuses before sending, so a 409 here means a stale view —
	// it must still read as a localized message, not the backend's English text.
	it('maps the task_blocked code to the localized message', () => {
		const err = new ApiError('task_blocked', 'task is blocked by an incomplete task', 409);
		expect(describeError(err, 'Fallback')).toBe(
			'Cannot complete: this task is blocked by an unfinished task'
		);
	});
});

describe('isBlocked', () => {
	function task(extra: Partial<Task>): Task {
		return { id: 1, blockedByCount: 0, relationCount: 0, ...extra } as Task;
	}

	it('is true when at least one open task blocks it', () => {
		expect(isBlocked(task({ blockedByCount: 1 }))).toBe(true);
		expect(isBlocked(task({ blockedByCount: 3 }))).toBe(true);
	});

	it('is false with no open blockers, even when relations exist', () => {
		expect(isBlocked(task({ blockedByCount: 0, relationCount: 4 }))).toBe(false);
	});

	// A Task synthesized from a pre-upgrade offline-cache entry has no count at all;
	// treating that as blocked would wedge the checkbox for every such task.
	it('is false when the field is absent (older cached shape)', () => {
		expect(isBlocked({ id: 1 } as Task)).toBe(false);
	});
});

describe('taskToJson', () => {
	const task = (id: number, parentId: number | null, title: string, description: string) =>
		({ id, parentId, title, description, priority: 'high', status: 'open' }) as Task;

	it('copies only title and description for a task without subtasks', () => {
		expect(taskToJson(task(1, null, 'Root', ''), [])).toEqual({
			title: 'Root',
			description: ''
		});
	});

	it('preserves the full nested subtree and sibling order without task metadata', () => {
		const root = task(1, null, 'Root', 'Root description');
		const first = task(2, 1, 'First', 'First description');
		const second = task(3, 1, 'Second', '');
		const grandchild = task(4, 2, 'Grandchild', 'Nested description');

		expect(taskToJson(root, [first, second, grandchild])).toEqual({
			title: 'Root',
			description: 'Root description',
			subtasks: [
				{
					title: 'First',
					description: 'First description',
					subtasks: [{ title: 'Grandchild', description: 'Nested description' }]
				},
				{ title: 'Second', description: '' }
			]
		});
	});
});
