import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/errors';
import { describeError } from './taskActions';

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
