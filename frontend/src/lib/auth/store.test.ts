import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthStore } from './store.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function emptyResponse(status = 204): Response {
	return new Response(null, { status });
}

describe('AuthStore', () => {
	afterEach(() => vi.restoreAllMocks());

	it('bootstrap → setup-required=true sets status=guest and setupRequired=true', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(jsonResponse({ required: true }));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: true, authenticated: false });
		expect(store.status).toBe('guest');
		expect(store.setupRequired).toBe(true);
		expect(store.user).toBeNull();
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('bootstrap → setup-required=false, refresh ok, /auth/me returns user → authenticated', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockResolvedValueOnce(jsonResponse({ access: 'A', refresh: 'R' }))
			.mockResolvedValueOnce(jsonResponse({ user: { id: 1, username: 'eu' } }));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: true });
		expect(store.status).toBe('authenticated');
		expect(store.accessToken).toBe('A');
		expect(store.user).toEqual({ id: 1, username: 'eu' });
	});

	it('bootstrap → refresh 401 sets guest', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockResolvedValueOnce(emptyResponse(401));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: false });
		expect(store.status).toBe('guest');
		expect(store.user).toBeNull();
		expect(store.accessToken).toBeNull();
	});

	it('login stores access + user and flips status to authenticated', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu' } })
		);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.login({ username: 'eu', password: 'p' });

		expect(store.status).toBe('authenticated');
		expect(store.accessToken).toBe('A');
		expect(store.user).toEqual({ id: 1, username: 'eu' });

		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect(init.method).toBe('POST');
		expect(init.credentials).toBe('include');
		expect(init.body).toBe(
			JSON.stringify({ username: 'eu', password: 'p', clientKind: 'web' })
		);
	});

	it('logout clears state even when API call fails', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		store.user = { id: 1, username: 'eu' };
		store.accessToken = 'A';
		store.status = 'authenticated';

		await store.logout();

		expect(store.status).toBe('guest');
		expect(store.user).toBeNull();
		expect(store.accessToken).toBeNull();
	});

	it('setup performs setup and authenticates', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu' } })
		);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		store.setupRequired = true;
		await store.setup({ username: 'eu', password: 'p' });

		expect(store.status).toBe('authenticated');
		expect(store.setupRequired).toBe(false);
		expect(store.user).toEqual({ id: 1, username: 'eu' });
	});
});
