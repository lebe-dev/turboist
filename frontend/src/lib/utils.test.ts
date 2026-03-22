import { describe, it, expect } from 'vitest';
import { incrementDuplicateTitle } from './utils';

describe('incrementDuplicateTitle', () => {
	it('increments trailing (N)', () => {
		expect(incrementDuplicateTitle('Buy milk (1)')).toBe('Buy milk (2)');
		expect(incrementDuplicateTitle('Task (7)')).toBe('Task (8)');
		expect(incrementDuplicateTitle('Task (99)')).toBe('Task (100)');
	});

	it('leaves title unchanged when no trailing (N)', () => {
		expect(incrementDuplicateTitle('Buy milk')).toBe('Buy milk');
		expect(incrementDuplicateTitle('Task with (parens) inside')).toBe('Task with (parens) inside');
	});

	it('handles (0)', () => {
		expect(incrementDuplicateTitle('Task (0)')).toBe('Task (1)');
	});

	it('ignores non-numeric parentheses at end', () => {
		expect(incrementDuplicateTitle('Task (abc)')).toBe('Task (abc)');
	});

	it('handles trailing whitespace after (N)', () => {
		expect(incrementDuplicateTitle('Task (3)  ')).toBe('Task (4)');
	});
});
