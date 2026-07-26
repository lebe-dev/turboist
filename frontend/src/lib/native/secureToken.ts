import { SecureStorage } from '@aparajita/capacitor-secure-storage';

// Persists the rotating refresh token in the iOS Keychain / Android Keystore-
// backed store. Web never uses this (its refresh token lives in an HttpOnly
// cookie) — AuthStore only wires a token store in when running natively.

export interface RefreshTokenStore {
	get(): Promise<string | null>;
	set(token: string | null): Promise<void>;
}

const KEY = 'refreshToken';

// @aparajita/capacitor-secure-storage uses POSITIONAL args: get(key),
// set(key, value), remove(key). get() resolves to null when the key is absent;
// only genuine backend failures throw, which we swallow to null so a missing/
// corrupt token degrades to "logged out" rather than crashing bootstrap.
export const nativeRefreshTokenStore: RefreshTokenStore = {
	async get(): Promise<string | null> {
		try {
			const value = await SecureStorage.get(KEY);
			return typeof value === 'string' ? value : null;
		} catch {
			return null;
		}
	},
	async set(token: string | null): Promise<void> {
		if (token === null) {
			await SecureStorage.remove(KEY);
			return;
		}
		await SecureStorage.set(KEY, token);
	}
};
