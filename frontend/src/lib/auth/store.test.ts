import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthStore } from './store.svelte';
import { decideAuthRedirect } from './guard';
import type { RefreshTokenStore } from '../native/secureToken';

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

	it('bootstrap → refresh fails + /config returns 503 setup_required → setupRequired=true', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(emptyResponse(401))
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'setup_required', message: 'setup required' } }, 503)
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: true, authenticated: false });
		expect(store.status).toBe('guest');
		expect(store.setupRequired).toBe(true);
		expect(store.user).toBeNull();
	});

	// Since v1.15 the refresh response embeds the user, so boot renders after a
	// single round-trip instead of a serial /auth/refresh → /auth/me chain.
	it('bootstrap → refresh carries the user → authenticated with no /auth/me call', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu' } })
		);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: true });
		expect(store.status).toBe('authenticated');
		expect(store.user).toEqual({ id: 1, username: 'eu' });
		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(String(fetchMock.mock.calls[0][0])).toContain('/auth/refresh');
	});

	// A new bundle can be talking to a not-yet-restarted older server, whose
	// refresh response has no `user` — the /auth/me fallback must stay live.
	it('bootstrap → refresh ok, /auth/me returns user → authenticated', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ access: 'A', refresh: 'R' }))
			.mockResolvedValueOnce(jsonResponse({ user: { id: 1, username: 'eu' } }));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: true });
		expect(store.status).toBe('authenticated');
		expect(store.accessToken).toBe('A');
		expect(store.user).toEqual({ id: 1, username: 'eu' });
	});

	it('bootstrap → refresh 401 + /config 401 → plain guest', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(emptyResponse(401))
			.mockResolvedValueOnce(emptyResponse(401));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: false });
		expect(store.status).toBe('guest');
		expect(store.user).toBeNull();
		expect(store.accessToken).toBeNull();
		expect(store.setupRequired).toBe(false);
	});

	it('login stores access + user and flips status to authenticated', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu' } })
		);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.login({ username: 'eu', password: 'p' });

		expect(result).toEqual({ otpRequired: false });
		expect(store.status).toBe('authenticated');
		expect(store.accessToken).toBe('A');
		expect(store.user).toEqual({ id: 1, username: 'eu' });
		expect(store.awaitingOtp).toBe(false);

		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect(init.method).toBe('POST');
		expect(init.credentials).toBe('include');
		expect(init.body).toBe(
			JSON.stringify({ username: 'eu', password: 'p', clientKind: 'web' })
		);
	});

	it('login with otpRequired keeps status=guest and flips awaitingOtp', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(jsonResponse({ otpRequired: true, ticket: 'TICKET' }));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.login({ username: 'eu', password: 'p' });

		expect(result).toEqual({ otpRequired: true });
		expect(store.awaitingOtp).toBe(true);
		expect(store.status).not.toBe('authenticated');
		expect(store.accessToken).toBeNull();
		expect(store.user).toBeNull();
	});

	it('verifyOtp posts ticket+code and finalises authentication', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ otpRequired: true, ticket: 'TICKET' }))
			.mockResolvedValueOnce(
				jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu', totpEnabled: true } })
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.login({ username: 'eu', password: 'p' });
		await store.verifyOtp('123456');

		expect(store.status).toBe('authenticated');
		expect(store.accessToken).toBe('A');
		expect(store.user).toEqual({ id: 1, username: 'eu', totpEnabled: true });
		expect(store.awaitingOtp).toBe(false);

		const otpInit = fetchMock.mock.calls[1][1] as RequestInit;
		expect(otpInit.method).toBe('POST');
		expect(otpInit.credentials).toBe('include');
		expect(otpInit.body).toBe(JSON.stringify({ ticket: 'TICKET', code: '123456' }));
		const url = fetchMock.mock.calls[1][0] as string;
		expect(url).toContain('/auth/login/otp');
	});

	it('verifyOtp throws when no challenge is in progress', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });

		await expect(store.verifyOtp('123456')).rejects.toThrow('No OTP challenge in progress');
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it('verifyOtp does not retry on failure but keeps awaitingOtp so the user can re-enter', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ otpRequired: true, ticket: 'TICKET' }))
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'totp_invalid_code', message: 'bad' } }, 401)
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.login({ username: 'eu', password: 'p' });
		await expect(store.verifyOtp('000000')).rejects.toBeDefined();

		expect(store.awaitingOtp).toBe(true);
		expect(store.status).not.toBe('authenticated');
	});

	it('cancelOtp clears the pending challenge', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(jsonResponse({ otpRequired: true, ticket: 'TICKET' }));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.login({ username: 'eu', password: 'p' });
		expect(store.awaitingOtp).toBe(true);

		store.cancelOtp();
		expect(store.awaitingOtp).toBe(false);
		await expect(store.verifyOtp('123456')).rejects.toThrow('No OTP challenge in progress');
	});

	it('logout clears state even when API call fails', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		store.user = { id: 1, username: 'eu', totpEnabled: false };
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

	// §4.9 — boot without network: /auth/refresh fails by network error (status 0).
	it('bootstrap → refresh network error → offlineSession, renders from cache, no /login', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		// A single rejected /auth/refresh; NO follow-up /config probe must be made.
		fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		const result = await store.bootstrap();

		expect(store.offlineSession).toBe(true);
		// Stays 'authenticated' so the (app) shell renders the cached workspace.
		expect(store.status).toBe('authenticated');
		expect(store.user).toBeNull();
		expect(result.authenticated).toBe(false);
		// Server unreachable: we must NOT probe /config after the network failure.
		expect(fetchMock).toHaveBeenCalledTimes(1);
		// The redirect guard must keep the user on their current route.
		expect(decideAuthRedirect(store, '/today')).toBeNull();
	});

	// §4.9 — an explicit 401 from /auth/refresh is a real rejection → guest → /login.
	it('bootstrap → refresh 401 → guest, offlineSession stays false, redirects to /login', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(emptyResponse(401))
			.mockResolvedValueOnce(emptyResponse(401));

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.bootstrap();

		expect(store.offlineSession).toBe(false);
		expect(store.status).toBe('guest');
		expect(decideAuthRedirect(store, '/today')).toBe('/login');
	});

	// §4.9 — native: a stored refresh token that fails by network must NOT be
	// cleared (the session survives the offline boot); we enter an offline session.
	it('bootstrap (native) → stored token + network error → offlineSession, token kept', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));
		const set = vi.fn<RefreshTokenStore['set']>().mockResolvedValue();
		const tokenStore: RefreshTokenStore = {
			get: vi.fn<RefreshTokenStore['get']>().mockResolvedValue('RT'),
			set
		};

		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			clientKind: 'ios',
			tokenStore
		});
		await store.bootstrap();

		expect(store.offlineSession).toBe(true);
		expect(store.status).toBe('authenticated');
		// The dead-token clear (set(null)) must NOT run on a network error.
		expect(set).not.toHaveBeenCalled();
	});

	// §4.9 — native: a stored token rejected by the server (401) IS dead → cleared.
	it('bootstrap (native) → stored token rejected 401 → guest, token cleared', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(emptyResponse(401)) // /auth/refresh
			.mockResolvedValueOnce(emptyResponse(401)); // /config probe
		const set = vi.fn<RefreshTokenStore['set']>().mockResolvedValue();
		const tokenStore: RefreshTokenStore = {
			get: vi.fn<RefreshTokenStore['get']>().mockResolvedValue('RT'),
			set
		};

		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			clientKind: 'ios',
			tokenStore
		});
		await store.bootstrap();

		expect(store.offlineSession).toBe(false);
		expect(store.status).toBe('guest');
		expect(set).toHaveBeenCalledWith(null);
	});
});
