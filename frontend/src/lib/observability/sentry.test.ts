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
