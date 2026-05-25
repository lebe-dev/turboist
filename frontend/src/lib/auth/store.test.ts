import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthStore } from './store.svelte';
import type { OfflineAuthAdapter } from '../offline/auth';
import type { User } from '../api/types';

interface OfflineFake {
	adapter: OfflineAuthAdapter;
	saved: User[];
	cleared: number;
	authenticatedRefreshes: number;
	stored: { id: number; user: User } | null;
	dataPresent: boolean;
}

const makeOfflineFake = (
	initial: { stored?: { id: number; user: User } | null; dataPresent?: boolean } = {}
): OfflineFake => {
	const state: OfflineFake = {
		adapter: undefined as unknown as OfflineAuthAdapter,
		saved: [],
		cleared: 0,
		authenticatedRefreshes: 0,
		stored: initial.stored ?? null,
		dataPresent: initial.dataPresent ?? false
	};
	state.adapter = {
		async saveUser(user) {
			state.saved.push(user);
			state.stored = { id: user.id, user };
		},
		async loadUser() {
			return state.stored;
		},
		async hasData() {
			return state.dataPresent;
		},
		async clear() {
			state.cleared += 1;
			state.stored = null;
			state.dataPresent = false;
		},
		async onAuthenticatedRefresh() {
			state.authenticatedRefreshes += 1;
		}
	};
	return state;
};

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

	it('bootstrap → refresh network error + offline data → authenticates from cache without token', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockRejectedValueOnce(new TypeError('offline'));

		const offline = makeOfflineFake({
			stored: { id: 7, user: { id: 7, username: 'eu', totpEnabled: false } },
			dataPresent: true
		});

		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: true });
		expect(store.status).toBe('authenticated');
		expect(store.user).toEqual({ id: 7, username: 'eu', totpEnabled: false });
		expect(store.accessToken).toBeNull();
		expect(offline.cleared).toBe(0);
		expect(offline.authenticatedRefreshes).toBe(0);
	});

	it('bootstrap → refresh network error + no offline data → guest', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockRejectedValueOnce(new TypeError('offline'));

		const offline = makeOfflineFake();
		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});
		const result = await store.bootstrap();

		expect(result).toEqual({ setupRequired: false, authenticated: false });
		expect(store.status).toBe('guest');
		expect(offline.cleared).toBe(0);
	});

	it('bootstrap → refresh 401 wipes cached offline data', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockResolvedValueOnce(emptyResponse(401));

		const offline = makeOfflineFake({
			stored: { id: 7, user: { id: 7, username: 'eu', totpEnabled: false } },
			dataPresent: true
		});
		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});
		await store.bootstrap();

		expect(store.status).toBe('guest');
		expect(offline.cleared).toBe(1);
	});

	it('bootstrap → refresh ok triggers onAuthenticatedRefresh and saves user', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(jsonResponse({ required: false }))
			.mockResolvedValueOnce(jsonResponse({ access: 'A', refresh: 'R' }))
			.mockResolvedValueOnce(jsonResponse({ user: { id: 9, username: 'eu', totpEnabled: false } }));

		const offline = makeOfflineFake();
		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});
		await store.bootstrap();

		expect(store.status).toBe('authenticated');
		expect(offline.saved).toEqual([{ id: 9, username: 'eu', totpEnabled: false }]);
		expect(offline.authenticatedRefreshes).toBe(1);
	});

	it('login persists user into offline storage', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu', totpEnabled: false } })
		);
		const offline = makeOfflineFake();
		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});

		await store.login({ username: 'eu', password: 'p' });

		expect(offline.saved).toEqual([{ id: 1, username: 'eu', totpEnabled: false }]);
	});

	it('logout clears offline data even when API call fails', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));
		const offline = makeOfflineFake({
			stored: { id: 1, user: { id: 1, username: 'eu', totpEnabled: false } },
			dataPresent: true
		});
		const store = new AuthStore({
			fetchImpl: fetchMock as unknown as typeof fetch,
			offline: offline.adapter
		});
		store.user = { id: 1, username: 'eu', totpEnabled: false };
		store.accessToken = 'A';
		store.status = 'authenticated';

		await store.logout();

		expect(store.status).toBe('guest');
		expect(offline.cleared).toBe(1);
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
