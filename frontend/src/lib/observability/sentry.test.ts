import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/api/errors';

const initMock = vi.fn();

vi.mock('@sentry/sveltekit', () => ({
	init: (...args: unknown[]) => initMock(...args)
}));

vi.mock('$lib/native/platform', () => ({
	isNativePlatform: () => false
}));

vi.mock('$lib/native/serverUrl', () => ({
	getServerUrl: () => ''
}));

describe('initSentry beforeSend', () => {
	beforeEach(() => {
		initMock.mockClear();
		vi.stubGlobal('__APP_VERSION__', 'test');
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ sentry: { dsn: 'https://example.test/1' } }), {
					status: 200,
					headers: { 'Content-Type': 'application/json' }
				})
			)
		);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.resetModules();
	});

	it('drops offline network_error reports so nothing is sent to Sentry while offline', async () => {
		const { initSentry } = await import('./sentry');
		await initSentry();

		expect(initMock).toHaveBeenCalledOnce();
		const config = initMock.mock.calls[0][0] as {
			beforeSend: (event: object, hint: { originalException?: unknown }) => object | null;
		};

		const event = { message: 'offline' };
		const offlineErr = new ApiError('network_error', 'network error', 0);
		expect(config.beforeSend(event, { originalException: offlineErr })).toBeNull();
	});

	it('still reports unrelated errors', async () => {
		const { initSentry } = await import('./sentry');
		await initSentry();

		const config = initMock.mock.calls[0][0] as {
			beforeSend: (event: object, hint: { originalException?: unknown }) => object | null;
		};

		const event = { message: 'boom' };
		expect(config.beforeSend(event, { originalException: new Error('boom') })).toBe(event);
	});
});

describe('initSentry singleflight', () => {
	beforeEach(() => {
		initMock.mockClear();
		vi.stubGlobal('__APP_VERSION__', 'test');
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.resetModules();
	});

	function stubConfigFetch(body: unknown): ReturnType<typeof vi.fn> {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify(body), {
				status: 200,
				headers: { 'Content-Type': 'application/json' }
			})
		);
		vi.stubGlobal('fetch', fetchMock);
		return fetchMock;
	}

	// The real boot does exactly this: hooks.client.ts fires initSentry() without
	// awaiting it, then +layout.svelte awaits it while the first fetch is still in
	// flight. A guard set at the end of the body cannot latch in time.
	it('fetches /api/config once when two callers overlap', async () => {
		const fetchMock = stubConfigFetch({ sentry: { dsn: 'https://example.test/1' } });
		const { initSentry } = await import('./sentry');

		const first = initSentry();
		const second = initSentry();
		await Promise.all([first, second]);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(initMock).toHaveBeenCalledOnce();
	});

	// Regression: `initialized` was only set after a DSN was found, so a deploy
	// with Sentry disabled refetched /api/config on every single call.
	it('does not refetch on a later call when the DSN is blank', async () => {
		const fetchMock = stubConfigFetch({ sentry: { dsn: '' } });
		const { initSentry } = await import('./sentry');

		await initSentry();
		await initSentry();

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(initMock).not.toHaveBeenCalled();
	});

	it('resolves rather than rejecting when the config endpoint is unreachable', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('ECONNREFUSED')));
		const { initSentry } = await import('./sentry');

		await expect(initSentry()).resolves.toBeUndefined();
		await expect(initSentry()).resolves.toBeUndefined();
		expect(initMock).not.toHaveBeenCalled();
	});
});
