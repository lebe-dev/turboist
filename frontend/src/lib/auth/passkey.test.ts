import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthStore } from './store.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function buffer(bytes: number[]): ArrayBuffer {
	return new Uint8Array(bytes).buffer;
}

function stubAssertion(): void {
	vi.stubGlobal('navigator', {
		credentials: {
			get: vi.fn().mockResolvedValue({
				id: 'cred-id',
				rawId: buffer([1]),
				type: 'public-key',
				getClientExtensionResults: () => ({}),
				response: {
					clientDataJSON: buffer([2]),
					authenticatorData: buffer([3]),
					signature: buffer([4]),
					userHandle: buffer([5])
				}
			})
		}
	});
}

function stubAttestation(): void {
	vi.stubGlobal('navigator', {
		credentials: {
			create: vi.fn().mockResolvedValue({
				id: 'cred-id',
				rawId: buffer([1]),
				type: 'public-key',
				getClientExtensionResults: () => ({}),
				response: {
					clientDataJSON: buffer([2]),
					attestationObject: buffer([3])
				}
			})
		}
	});
}

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe('AuthStore.loginWithPasskey', () => {
	it('runs both ceremony halves and authenticates', async () => {
		stubAssertion();
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({ ceremonyId: 'C1', options: { publicKey: { challenge: 'AQID' } } })
			)
			.mockResolvedValueOnce(
				jsonResponse({ access: 'A', refresh: 'R', user: { id: 1, username: 'eu' } })
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await store.loginWithPasskey();

		expect(String(fetchMock.mock.calls[0][0])).toContain('/auth/passkey/login/begin');
		const finishUrl = String(fetchMock.mock.calls[1][0]);
		expect(finishUrl).toContain('/auth/passkey/login/finish');

		// The ceremony id ties the assertion back to the server-side challenge;
		// dropping it would make every login unverifiable.
		const finishBody = JSON.parse(String(fetchMock.mock.calls[1][1]?.body));
		expect(finishBody.ceremonyId).toBe('C1');
		expect(finishBody.clientKind).toBe('web');
		expect(finishBody.credential.response.userHandle).toBe('BQ');

		expect(store.status).toBe('authenticated');
		expect(store.user).toEqual({ id: 1, username: 'eu' });
		expect(store.accessToken).toBe('A');
	});

	it('leaves the session untouched when the server rejects the assertion', async () => {
		stubAssertion();
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({ ceremonyId: 'C1', options: { publicKey: { challenge: 'AQID' } } })
			)
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'auth_invalid', message: 'invalid credentials' } }, 401)
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		await expect(store.loginWithPasskey()).rejects.toMatchObject({ code: 'auth_invalid' });
		expect(store.status).toBe('loading');
		expect(store.user).toBeNull();
	});
});

describe('AuthStore.registerPasskey', () => {
	it('posts the attestation with the chosen name and returns the stored passkey', async () => {
		stubAttestation();
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({
					ceremonyId: 'C2',
					options: {
						publicKey: {
							challenge: 'AQID',
							user: { id: 'BAUG', name: 'eu', displayName: 'eu' }
						}
					}
				})
			)
			.mockResolvedValueOnce(
				jsonResponse(
					{ id: 9, name: 'MacBook', createdAt: '2026-08-18T00:00:00.000Z', lastUsedAt: null },
					201
				)
			);

		const store = new AuthStore({ fetchImpl: fetchMock as unknown as typeof fetch });
		store.accessToken = 'A';
		const created = await store.registerPasskey('MacBook');

		expect(String(fetchMock.mock.calls[0][0])).toContain('/api/v1/passkeys/register/begin');
		const finishBody = JSON.parse(String(fetchMock.mock.calls[1][1]?.body));
		expect(finishBody).toMatchObject({ ceremonyId: 'C2', name: 'MacBook' });
		expect(finishBody.credential.response.attestationObject).toBe('Aw');
		expect(created).toMatchObject({ id: 9, name: 'MacBook' });
	});
});
