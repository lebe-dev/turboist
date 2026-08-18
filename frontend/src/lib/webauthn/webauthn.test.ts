import { describe, expect, it, vi, afterEach } from 'vitest';
import { base64UrlToBytes, bytesToBase64Url } from './encoding';
import { createPasskeyCredential, getPasskeyAssertion, isPasskeyCancellation } from './index';

function buffer(bytes: number[]): ArrayBuffer {
	return new Uint8Array(bytes).buffer;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('base64url encoding', () => {
	it('round-trips binary data', () => {
		const bytes = new Uint8Array([0, 1, 250, 251, 252, 253, 254, 255]);
		const encoded = bytesToBase64Url(bytes.buffer);
		expect(encoded).not.toMatch(/[+/=]/);
		expect(Array.from(base64UrlToBytes(encoded))).toEqual(Array.from(bytes));
	});

	it('decodes values without padding', () => {
		// The server emits unpadded base64url; decoding must not depend on '='.
		expect(Array.from(base64UrlToBytes('AQ'))).toEqual([1]);
		expect(Array.from(base64UrlToBytes('AQI'))).toEqual([1, 2]);
	});
});

describe('createPasskeyCredential', () => {
	it('decodes the options and re-encodes the attestation', async () => {
		const create = vi.fn().mockResolvedValue({
			id: 'cred-id',
			rawId: buffer([1, 2, 3]),
			type: 'public-key',
			authenticatorAttachment: 'platform',
			getClientExtensionResults: () => ({ credProps: { rk: true } }),
			response: {
				clientDataJSON: buffer([4, 5]),
				attestationObject: buffer([6, 7]),
				getTransports: () => ['internal', 'hybrid']
			}
		});
		vi.stubGlobal('navigator', { credentials: { create } });

		const credential = await createPasskeyCredential({
			publicKey: {
				challenge: 'AQID',
				rp: { id: 'example.com', name: 'Turboist' },
				user: { id: 'BAUG', name: 'admin', displayName: 'admin' },
				excludeCredentials: [{ id: 'AQ', type: 'public-key', transports: ['internal'] }]
			}
		});

		// The browser API only speaks ArrayBuffers, so every binary option must
		// arrive decoded.
		const passed = create.mock.calls[0][0].publicKey;
		expect(Array.from(new Uint8Array(passed.challenge))).toEqual([1, 2, 3]);
		expect(Array.from(new Uint8Array(passed.user.id))).toEqual([4, 5, 6]);
		expect(Array.from(new Uint8Array(passed.excludeCredentials[0].id))).toEqual([1]);

		expect(credential).toMatchObject({
			id: 'cred-id',
			rawId: 'AQID',
			type: 'public-key',
			authenticatorAttachment: 'platform',
			response: {
				clientDataJSON: 'BAU',
				attestationObject: 'Bgc',
				transports: ['internal', 'hybrid']
			}
		});
	});

	it('throws when the platform returns no credential', async () => {
		vi.stubGlobal('navigator', { credentials: { create: vi.fn().mockResolvedValue(null) } });
		await expect(
			createPasskeyCredential({
				publicKey: { challenge: 'AQ', user: { id: 'AQ', name: 'a', displayName: 'a' } }
			})
		).rejects.toThrow();
	});
});

describe('getPasskeyAssertion', () => {
	it('encodes the assertion, including the user handle', async () => {
		const get = vi.fn().mockResolvedValue({
			id: 'cred-id',
			rawId: buffer([1]),
			type: 'public-key',
			authenticatorAttachment: null,
			getClientExtensionResults: () => ({}),
			response: {
				clientDataJSON: buffer([2]),
				authenticatorData: buffer([3]),
				signature: buffer([4]),
				// The handle is what makes the login usernameless — it must survive
				// the round-trip to the server.
				userHandle: buffer([5])
			}
		});
		vi.stubGlobal('navigator', { credentials: { get } });

		const credential = await getPasskeyAssertion({
			publicKey: { challenge: 'AQID', rpId: 'example.com' }
		});

		const passed = get.mock.calls[0][0].publicKey;
		expect(Array.from(new Uint8Array(passed.challenge))).toEqual([1, 2, 3]);
		expect(passed.allowCredentials).toEqual([]);
		expect(credential).toMatchObject({
			rawId: 'AQ',
			response: {
				clientDataJSON: 'Ag',
				authenticatorData: 'Aw',
				signature: 'BA',
				userHandle: 'BQ'
			}
		});
	});

	it('omits the user handle when the authenticator returns none', async () => {
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
						userHandle: null
					}
				})
			}
		});

		const credential = (await getPasskeyAssertion({ publicKey: { challenge: 'AQ' } })) as {
			response: { userHandle?: string };
		};
		expect(credential.response.userHandle).toBeUndefined();
	});
});

describe('isPasskeyCancellation', () => {
	it('recognises a dismissed prompt', () => {
		expect(isPasskeyCancellation(new DOMException('nope', 'NotAllowedError'))).toBe(true);
		expect(isPasskeyCancellation(new DOMException('nope', 'AbortError'))).toBe(true);
	});

	it('does not swallow real failures', () => {
		expect(isPasskeyCancellation(new DOMException('boom', 'SecurityError'))).toBe(false);
		expect(isPasskeyCancellation(new Error('network'))).toBe(false);
	});
});
