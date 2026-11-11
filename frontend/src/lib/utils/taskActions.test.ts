import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/errors';
import type { Label, Task } from '$lib/api/types';
import {
	describeError,
	hasTitleCounter,
	isBlocked,
	taskToJson,
	tomorrowDuplicateDue
} from './taskActions';

describe('describeError', () => {
	it('returns the ApiError message when present', () => {
		const err = new ApiError('validation_failed', 'Title is required', 422);
		expect(describeError(err, 'Fallback')).toBe('Title is required');
	});

	it('falls back when the ApiError has an empty message', () => {
		const err = new ApiError('internal_error', '', 500);
		expect(describeError(err, 'Fallback')).toBe('Fallback');
	});

	it('explains a write the Today lock refused instead of the backend wording', () => {
		const err = new ApiError(
			'today_locked',
			'today is locked to the projects already planned for it',
			409,
			{
				lockedUntil: '2026-11-11T21:00:00.000Z',
				projectId: 4
			}
		);
		expect(describeError(err, 'Fallback')).toBe(
			'Until midnight, today takes only today’s projects. Pick another day.'
		);
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
			"No connection — this page hasn't been opened online yet, so there's nothing cached to show."
		);
		expect(describeError(new ApiError('timeout', 'request timed out', 0), 'Fallback')).toBe(
			"No connection — this page hasn't been opened online yet, so there's nothing cached to show."
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
	const label = (id: number, name: string, description: string) =>
		({ id, name, description, color: 'blue', isFavourite: false, isPrivate: false }) as Label;
	const task = (
		id: number,
		parentId: number | null,
		title: string,
		description: string,
		labels: Label[] = []
	) => ({ id, parentId, title, description, labels, priority: 'high', status: 'open' }) as Task;

	it('copies title, description and an empty label list for a bare task', () => {
		expect(taskToJson(task(1, null, 'Root', ''), [])).toEqual({
			title: 'Root',
			description: '',
			labels: [],
			labelGlossary: []
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
			labels: [],
			subtasks: [
				{
					title: 'First',
					description: 'First description',
					labels: [],
					subtasks: [{ title: 'Grandchild', description: 'Nested description', labels: [] }]
				},
				{ title: 'Second', description: '', labels: [] }
			],
			labelGlossary: []
		});
	});

	it('names labels on every task and describes each one once in the root glossary', () => {
		const urgent = label(1, 'urgent', 'Must ship today');
		const backend = label(2, 'backend', '');
		const root = task(1, null, 'Root', '', [urgent]);
		const child = task(2, 1, 'Child', '', [backend, urgent]);

		const json = taskToJson(root, [child]);

		expect(json.labels).toEqual(['urgent']);
		expect(json.subtasks?.[0].labels).toEqual(['backend', 'urgent']);
		expect(json.labelGlossary).toEqual([
			{ name: 'backend', description: '' },
			{ name: 'urgent', description: 'Must ship today' }
		]);
		expect(Object.keys(json)).toEqual([
			'title',
			'description',
			'labels',
			'subtasks',
			'labelGlossary'
		]);
	});

	it('prefers the freshest known description over the one embedded in the task', () => {
		const stale = label(1, 'urgent', 'old text');
		const fresh = label(1, 'urgent', 'new text');

		expect(taskToJson(task(1, null, 'Root', '', [stale]), [], [fresh]).labelGlossary).toEqual([
			{ name: 'urgent', description: 'new text' }
		]);
	});
});

describe('hasTitleCounter', () => {
	it('detects a trailing number in parentheses', () => {
		expect(hasTitleCounter('Read a chapter (3)')).toBe(true);
		expect(hasTitleCounter('Run (12)')).toBe(true);
	});

	it('ignores titles without a trailing counter', () => {
		expect(hasTitleCounter('Read a chapter')).toBe(false);
		expect(hasTitleCounter('Read (3) chapters')).toBe(false);
		expect(hasTitleCounter('(3)')).toBe(false);
		expect(hasTitleCounter('Read (three)')).toBe(false);
	});
});

describe('tomorrowDuplicateDue', () => {
	const now = new Date('2026-11-08T10:00:00Z');

	it('puts an undated task on the start of tomorrow', () => {
		expect(tomorrowDuplicateDue(null, false, now, 'UTC')).toEqual({
			dueAt: '2026-11-09T00:00:00.000Z',
			dueHasTime: false
		});
	});

	it('keeps the time of day of a timed task', () => {
		expect(tomorrowDuplicateDue('2026-11-08T07:30:00.000Z', true, now, 'UTC')).toEqual({
			dueAt: '2026-11-09T07:30:00.000Z',
			dueHasTime: true
		});
	});

	it('reads the time of day in the configured timezone', () => {
		// 07:30 in Moscow (UTC+3) is 04:30 UTC; tomorrow Moscow day starts 2026-11-08T21:00Z.
		expect(tomorrowDuplicateDue('2026-11-08T04:30:00.000Z', true, now, 'Europe/Moscow')).toEqual({
			dueAt: '2026-11-09T04:30:00.000Z',
			dueHasTime: true
		});
	});
});
