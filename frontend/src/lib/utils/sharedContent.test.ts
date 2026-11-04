import { describe, expect, it } from 'vitest';
import { prefillFromShare } from './sharedContent';

describe('prefillFromShare', () => {
	it('uses the url field as the title', () => {
		expect(prefillFromShare({ title: 'Some page', url: 'https://example.com/a' })).toEqual({
			title: 'https://example.com/a',
			description: 'Some page'
		});
	});

	it('pulls the link out of text shared by Android apps', () => {
		expect(prefillFromShare({ text: 'Look at this: https://habr.com/ru/articles/1/.' })).toEqual({
			title: 'https://habr.com/ru/articles/1/',
			description: 'Look at this:'
		});
	});

	it('does not repeat the title in the description', () => {
		expect(prefillFromShare({ title: 'Video', text: 'Video https://youtu.be/x', url: '' })).toEqual(
			{ title: 'https://youtu.be/x', description: 'Video' }
		);
	});

	it('falls back to plain text when no link is shared', () => {
		expect(prefillFromShare({ title: 'Buy milk', text: 'two litres' })).toEqual({
			title: 'Buy milk',
			description: 'two litres'
		});
		expect(prefillFromShare({ text: 'Buy milk' })).toEqual({ title: 'Buy milk', description: '' });
	});

	it('returns null for an empty share', () => {
		expect(prefillFromShare({ title: ' ', text: null })).toBeNull();
	});
});
