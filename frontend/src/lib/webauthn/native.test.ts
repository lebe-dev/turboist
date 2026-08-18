import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

const createCredential = vi.fn();
const getCredential = vi.fn();
const getConfiguration = vi.fn();
const isNativePlatform = vi.fn();

vi.mock('@capacitor/core', () => ({
	Capacitor: {
		isNativePlatform: () => isNativePlatform(),
		getPlatform: () => 'android'
	}
}));

vi.mock('@capgo/capacitor-passkey', () => ({
	CapacitorPasskey: {
		createCredential: (o: unknown) => createCredential(o),
		getCredential: (o: unknown) => getCredential(o),
		getConfiguration: () => getConfiguration()
	}
}));

const { createPasskeyCredential, getPasskeyAssertion } = await import('./index');

beforeEach(() => {
	isNativePlatform.mockReturnValue(true);
	getConfiguration.mockResolvedValue({ origin: 'https://todo.example.com', domains: [] });
	createCredential.mockResolvedValue({ id: 'c', rawId: 'c', type: 'public-key', response: {} });
	getCredential.mockResolvedValue({ id: 'c', rawId: 'c', type: 'public-key', response: {} });
});

afterEach(() => vi.clearAllMocks());

describe('native passkey ceremonies', () => {
	// The plugin tells an assertion from a registration by whether `mediation` is
	// present. Dropping it made every native login run the create path, which
	// reads `publicKey.rp.id` — absent from login options — and threw
	// "Cannot read properties of undefined (reading 'id')".
	it('marks an assertion with mediation', async () => {
		await getPasskeyAssertion({ publicKey: { challenge: 'AQID', rpId: 'todo.example.com' } });

		const passed = getCredential.mock.calls[0][0] as Record<string, unknown>;
		expect('mediation' in passed).toBe(true);
		expect(passed.mediation).not.toBe('conditional');
		expect(passed.publicKey).toEqual({ challenge: 'AQID', rpId: 'todo.example.com' });
	});

	it('never marks a registration with mediation', async () => {
		await createPasskeyCredential({
			publicKey: {
				challenge: 'AQID',
				rp: { id: 'todo.example.com', name: 'Turboist' },
				user: { id: 'BAUG', name: 'admin', displayName: 'admin' }
			}
		});

		const passed = createCredential.mock.calls[0][0] as Record<string, unknown>;
		expect('mediation' in passed).toBe(false);
	});

	// iOS 17.4+ puts this origin into clientDataJSON; left to the plugin it would
	// fall back to the WebView's own origin, which the server refuses.
	it('claims the configured origin, not the WebView one', async () => {
		await getPasskeyAssertion({ publicKey: { challenge: 'AQ' } });
		await createPasskeyCredential({
			publicKey: { challenge: 'AQ', rp: { id: 'x', name: 'x' }, user: { id: 'AQ', name: 'a', displayName: 'a' } }
		});

		expect((getCredential.mock.calls[0][0] as { origin?: string }).origin).toBe(
			'https://todo.example.com'
		);
		expect((createCredential.mock.calls[0][0] as { origin?: string }).origin).toBe(
			'https://todo.example.com'
		);
	});

	it('still runs when the build has no passkey configuration', async () => {
		getConfiguration.mockRejectedValue(new Error('no config'));

		await getPasskeyAssertion({ publicKey: { challenge: 'AQ' } });

		expect((getCredential.mock.calls[0][0] as { origin?: string }).origin).toBeUndefined();
	});
});
