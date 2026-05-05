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
});
