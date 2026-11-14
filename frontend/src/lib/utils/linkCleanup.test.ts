import { describe, expect, it } from 'vitest';
import { stripTrackingParams } from './linkCleanup';

describe('stripTrackingParams', () => {
	it('leaves plain text and clean links alone', () => {
		expect(stripTrackingParams('buy milk')).toBe('buy milk');
		expect(stripTrackingParams('https://example.com/a?id=5')).toBe('https://example.com/a?id=5');
		expect(stripTrackingParams('')).toBe('');
	});

	it('removes utm and click id parameters', () => {
		expect(stripTrackingParams('https://example.com/a?utm_source=x&utm_term=y&id=5')).toBe(
			'https://example.com/a?id=5'
		);
		expect(stripTrackingParams('https://example.com/?gclid=1&fbclid=2&yclid=3&msclkid=4')).toBe(
			'https://example.com/'
		);
	});

	it('keeps the fragment and drops an emptied query', () => {
		expect(stripTrackingParams('https://example.com/a?utm_campaign=z#part')).toBe(
			'https://example.com/a#part'
		);
	});

	it('matches names case-insensitively but only whole names', () => {
		expect(stripTrackingParams('https://example.com/?UTM_Source=x&a=1')).toBe(
			'https://example.com/?a=1'
		);
		expect(stripTrackingParams('https://example.com/?utmx=1&my_utm_source=2')).toBe(
			'https://example.com/?utmx=1&my_utm_source=2'
		);
	});

	it('cleans links inside text and keeps trailing punctuation', () => {
		expect(stripTrackingParams('read https://example.com/a?utm_source=x later')).toBe(
			'read https://example.com/a later'
		);
		expect(stripTrackingParams('see https://example.com/a?utm_source=x.')).toBe(
			'see https://example.com/a.'
		);
		expect(stripTrackingParams('[post](https://example.com/a?utm_source=x&b=2)')).toBe(
			'[post](https://example.com/a?b=2)'
		);
	});

	it('does not re-encode what stays', () => {
		expect(stripTrackingParams('https://b.io/?q=%D1%82&fbclid=2')).toBe('https://b.io/?q=%D1%82');
	});
});
