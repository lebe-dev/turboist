import { describe, it, expect } from 'vitest';
import { cleanTrackingParams } from '$lib/utils';

describe('cleanTrackingParams', () => {
	it('strips utm_* params from a URL', () => {
		const url = 'https://example.com/page?utm_source=google&utm_medium=cpc&id=42';
		expect(cleanTrackingParams(url)).toBe('https://example.com/page?id=42');
	});

	it('strips fbclid', () => {
		const url = 'https://example.com/?fbclid=abc123';
		expect(cleanTrackingParams(url)).toBe('https://example.com/');
	});

	it('strips gclid and gclsrc', () => {
		const url = 'https://example.com/page?gclid=xyz&gclsrc=aw.ds&ref=home';
		expect(cleanTrackingParams(url)).toBe('https://example.com/page?ref=home');
	});

	it('strips YouTube si param', () => {
		const url = 'https://www.youtube.com/watch?v=dQw4w9WgXcQ&si=tracking123';
		expect(cleanTrackingParams(url)).toBe('https://www.youtube.com/watch?v=dQw4w9WgXcQ');
	});

	it('returns URL unchanged when no tracking params', () => {
		const url = 'https://example.com/page?id=42&sort=name';
		expect(cleanTrackingParams(url)).toBe(url);
	});

	it('cleans URLs embedded in text', () => {
		const text = 'Check this: https://example.com/?utm_source=test and also https://other.com/?id=1';
		expect(cleanTrackingParams(text)).toBe('Check this: https://example.com/ and also https://other.com/?id=1');
	});

	it('cleans markdown link URLs', () => {
		const text = '[Link](https://example.com/?utm_campaign=promo&ref=top)';
		expect(cleanTrackingParams(text)).toBe('[Link](https://example.com/?ref=top)');
	});

	it('handles URL with only tracking params', () => {
		const url = 'https://example.com/?utm_source=google&utm_medium=cpc';
		expect(cleanTrackingParams(url)).toBe('https://example.com/');
	});

	it('handles invalid URLs gracefully', () => {
		const text = 'not a url at all';
		expect(cleanTrackingParams(text)).toBe(text);
	});

	it('strips msclkid', () => {
		const url = 'https://example.com/?msclkid=abc&page=1';
		expect(cleanTrackingParams(url)).toBe('https://example.com/?page=1');
	});
});
