import { describe, it, expect } from 'vitest';
import { incrementDuplicateTitle, stripMarkdownLinks } from './utils';

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

describe('stripMarkdownLinks', () => {
	it('strips markdown link syntax keeping text', () => {
		expect(stripMarkdownLinks('[Magic Link](https://example.com)')).toBe('Magic Link');
	});

	it('strips multiple links', () => {
		expect(stripMarkdownLinks('Read [A](https://a.com) and [B](https://b.com)')).toBe('Read A and B');
	});

	it('leaves plain text unchanged', () => {
		expect(stripMarkdownLinks('No links here')).toBe('No links here');
	});

	it('handles links with query params', () => {
		expect(stripMarkdownLinks('[Title](https://example.com/path?a=1&b=2)')).toBe('Title');
	});
});
