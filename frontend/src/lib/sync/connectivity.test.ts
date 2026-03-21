import { describe, it, expect, vi, afterEach } from 'vitest';

describe('isOnline', () => {
	const originalFetch = globalThis.fetch;

	afterEach(() => {
		globalThis.fetch = originalFetch;
		Object.defineProperty(navigator, 'onLine', { value: true, configurable: true });
	});

	it('returns false when navigator.onLine is false', async () => {
		Object.defineProperty(navigator, 'onLine', { value: false, configurable: true });

		const { isOnline } = await import('./connectivity');
		expect(await isOnline()).toBe(false);
	});

	it('returns true when online and /api/health returns ok', async () => {
		Object.defineProperty(navigator, 'onLine', { value: true, configurable: true });
		globalThis.fetch = vi.fn(() => Promise.resolve(new Response(null, { status: 200 })));

		const { isOnline } = await import('./connectivity');
		expect(await isOnline()).toBe(true);
	});

	it('returns false when fetch throws', async () => {
		Object.defineProperty(navigator, 'onLine', { value: true, configurable: true });
		globalThis.fetch = vi.fn(() => Promise.reject(new Error('Network error')));

		const { isOnline } = await import('./connectivity');
		expect(await isOnline()).toBe(false);
	});
});
