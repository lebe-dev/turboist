import { describe, expect, it } from 'vitest';
import { cacheName, isNavigationRequest, isStaleCache, shouldBypass } from './routing';

describe('cacheName', () => {
	it('namespaces the cache by the deploy version', () => {
		expect(cacheName('abc123')).toBe('cache-abc123');
	});
});

describe('shouldBypass', () => {
	it('never intercepts API requests — data is the JS layer job (§5.1)', () => {
		expect(shouldBypass('/api/v1/tasks/today')).toBe(true);
		expect(shouldBypass('/api/v1/inbox/tasks')).toBe(true);
		expect(shouldBypass('/api/config')).toBe(true);
	});

	it('never intercepts the SSE stream or its ticket mint', () => {
		// GET /api/v1/events (stream) and POST /api/v1/events/ticket both sit
		// under /api/, so the prefix check alone leaves them to the network.
		expect(shouldBypass('/api/v1/events')).toBe(true);
		expect(shouldBypass('/api/v1/events/ticket')).toBe(true);
	});

	it('never intercepts auth endpoints', () => {
		expect(shouldBypass('/auth/refresh')).toBe(true);
		expect(shouldBypass('/auth/setup')).toBe(true);
	});

	it('intercepts the app shell, its assets and static files', () => {
		expect(shouldBypass('/')).toBe(false);
		expect(shouldBypass('/today')).toBe(false);
		expect(shouldBypass('/_app/immutable/entry/start.abc123.js')).toBe(false);
		expect(shouldBypass('/manifest.webmanifest')).toBe(false);
		expect(shouldBypass('/robots.txt')).toBe(false);
	});
});

describe('isNavigationRequest', () => {
	it('is true only for navigate-mode requests', () => {
		expect(isNavigationRequest({ mode: 'navigate' })).toBe(true);
		expect(isNavigationRequest({ mode: 'cors' })).toBe(false);
		expect(isNavigationRequest({ mode: 'no-cors' })).toBe(false);
		expect(isNavigationRequest({})).toBe(false);
	});
});

describe('isStaleCache', () => {
	it('flags every cache whose key is not the current deploy version', () => {
		expect(isStaleCache('cache-old', 'new')).toBe(true);
		expect(isStaleCache('cache-new', 'new')).toBe(false);
		expect(isStaleCache('some-unrelated-cache', 'new')).toBe(true);
	});
});
