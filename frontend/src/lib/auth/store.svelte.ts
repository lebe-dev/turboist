import { ApiClient, setApiClient } from '../api/client';
import { ApiError } from '../api/errors';
import { auth, type AuthCredentials } from '../api/endpoints/auth';
import type { User, ClientKind, AuthLoginSuccessResponse } from '../api/types';
import { addApiLogEntry } from '../stores/apiLog.svelte';
import { clientOrigin } from '../realtime/origin';
import { onSelfMutation } from '../realtime/selfRefresh';
import { isNativePlatform, resolveClientKind } from '../native/platform';
import { nativeRefreshTokenStore, type RefreshTokenStore } from '../native/secureToken';
import { getServerUrl } from '../native/serverUrl';

export type AuthStatus = 'loading' | 'guest' | 'authenticated';

export interface AuthStoreOptions {
	baseUrl?: string;
	fetchImpl?: typeof fetch;
	// Native only. Web leaves both unset: clientKind defaults to 'web' and the
	// refresh token stays in the HttpOnly cookie (no tokenStore).
	clientKind?: ClientKind;
	tokenStore?: RefreshTokenStore | null;
}

interface BootstrapResult {
	setupRequired: boolean;
	authenticated: boolean;
}

export class AuthStore {
	user = $state<User | null>(null);
	accessToken = $state<string | null>(null);
	status = $state<AuthStatus>('loading');
	setupRequired = $state<boolean>(false);
	awaitingOtp = $state<boolean>(false);

	// Ticket from /auth/login that authorises a follow-up /auth/login/otp.
	// Kept in memory only — never persisted to localStorage.
	private otpTicket: string | null = null;

	readonly client: ApiClient;
	private readonly clientKind: ClientKind;
	private readonly tokenStore: RefreshTokenStore | null;

	constructor(options: AuthStoreOptions = {}) {
		this.clientKind = options.clientKind ?? 'web';
		this.tokenStore = options.tokenStore ?? null;
		this.client = new ApiClient({
			baseUrl: options.baseUrl,
			fetchImpl: options.fetchImpl,
			getAccessToken: () => this.accessToken,
			setAccessToken: (token) => {
				this.accessToken = token;
			},
			onRefreshFailure: () => {
				this.user = null;
				this.accessToken = null;
				this.status = 'guest';
			},
			onLog: (entry) => addApiLogEntry(entry),
			clientOrigin,
			onMutation: (path) => onSelfMutation(path),
			getRefreshToken: this.tokenStore ? () => this.tokenStore!.get() : undefined,
			setRefreshToken: this.tokenStore ? (t) => this.tokenStore!.set(t) : undefined
		});
		setApiClient(this.client);
	}

	async bootstrap(): Promise<BootstrapResult> {
		this.status = 'loading';

		const refreshed = await this.tryRefresh();
		if (!refreshed) {
			// Refresh failed (no cookie or expired): probe /api/v1/config without
			// auth. SetupCheckMiddleware runs before the auth middleware on the
			// /api/v1 group, so an un-set-up instance returns 503 setup_required;
			// a set-up instance returns 401 (which we treat as plain guest).
			const setupNeeded = await this.probeSetupRequired();
			this.setupRequired = setupNeeded;
			this.status = 'guest';
			return { setupRequired: setupNeeded, authenticated: false };
		}

		try {
			const me = await auth.me(this.client);
			this.user = me.user;
			this.status = 'authenticated';
			return { setupRequired: false, authenticated: true };
		} catch {
			this.accessToken = null;
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}
	}

	// probeSetupRequired hits /api/v1/config without auth to learn whether the
	// instance still needs the initial admin user. The endpoint requires auth in
	// general; we only care about the SetupCheckMiddleware response in front of
	// it, which short-circuits with 503 setup_required when the DB has no users.
	private async probeSetupRequired(): Promise<boolean> {
		try {
			await this.client.fetch('/api/v1/config', { skipAuth: true, skipRefresh: true });
			return false;
		} catch (err) {
			if (err instanceof ApiError && err.code === 'setup_required') {
				return true;
			}
			return false;
		}
	}

	private async tryRefresh(): Promise<boolean> {
		try {
			let stored: string | undefined;
			if (this.tokenStore) {
				const rt = await this.tokenStore.get();
				// Native and logged out: no stored token → plain guest, no network call.
				if (!rt) return false;
				stored = rt;
			}
			const res = await auth.refresh(this.client, stored);
			this.accessToken = res.access;
			if (this.tokenStore && res.refresh) await this.tokenStore.set(res.refresh);
			return true;
		} catch (err) {
			if (this.tokenStore && err instanceof ApiError && err.status === 401) {
				// Dead token — clear it so we don't retry the same one every launch.
				await this.tokenStore.set(null);
			}
			return false;
		}
	}

	// finaliseAuth applies a successful login/setup/otp response. On native it
	// also persists the returned refresh token (there is no Set-Cookie for
	// non-web clients); on web tokenStore is null and the cookie was already set.
	private async finaliseAuth(res: AuthLoginSuccessResponse): Promise<void> {
		this.otpTicket = null;
		this.awaitingOtp = false;
		this.accessToken = res.access;
		this.user = res.user;
		this.status = 'authenticated';
		this.setupRequired = false;
		if (this.tokenStore && res.refresh) await this.tokenStore.set(res.refresh);
	}

	async login(
		credentials: Omit<AuthCredentials, 'clientKind'>
	): Promise<{ otpRequired: boolean }> {
		const res = await auth.login(this.client, { ...credentials, clientKind: this.clientKind });
		if ('otpRequired' in res) {
			this.otpTicket = res.ticket;
			this.awaitingOtp = true;
			return { otpRequired: true };
		}
		await this.finaliseAuth(res);
		return { otpRequired: false };
	}

	async verifyOtp(code: string): Promise<void> {
		if (!this.otpTicket) {
			throw new Error('No OTP challenge in progress');
		}
		const res = await auth.loginOtp(this.client, { ticket: this.otpTicket, code });
		await this.finaliseAuth(res);
	}

	cancelOtp(): void {
		this.otpTicket = null;
		this.awaitingOtp = false;
	}

	async setup(credentials: Omit<AuthCredentials, 'clientKind'>): Promise<void> {
		const res = await auth.setup(this.client, { ...credentials, clientKind: this.clientKind });
		await this.finaliseAuth(res);
	}

	async logout(): Promise<void> {
		try {
			await auth.logout(this.client);
		} catch {
			// best-effort; clear local state regardless
		}
		await this.clear();
	}

	async logoutAll(): Promise<void> {
		try {
			await auth.logoutAll(this.client);
		} catch {
			// best-effort
		}
		await this.clear();
	}

	// clear wipes in-memory auth state and, on native, the stored refresh token
	// (so a relaunch requires login). The server URL is left intact — it is
	// configuration, not a credential.
	private async clear(): Promise<void> {
		this.user = null;
		this.accessToken = null;
		this.status = 'guest';
		this.otpTicket = null;
		this.awaitingOtp = false;
		if (this.tokenStore) await this.tokenStore.set(null);
	}
}

let storeInstance: AuthStore | null = null;

export function createAuthStore(options: AuthStoreOptions = {}): AuthStore {
	// The factory is the ONLY place the platform is resolved, so unit tests that
	// construct `new AuthStore({...})` directly stay hermetic — web defaults,
	// never touching the Capacitor bridge.
	const resolved: AuthStoreOptions = {
		...options,
		clientKind: options.clientKind ?? resolveClientKind(),
		tokenStore: options.tokenStore ?? (isNativePlatform() ? nativeRefreshTokenStore : null),
		baseUrl: options.baseUrl ?? (isNativePlatform() ? getServerUrl() : undefined)
	};
	storeInstance = new AuthStore(resolved);
	return storeInstance;
}

export function getAuthStore(): AuthStore {
	if (!storeInstance) {
		throw new Error('AuthStore is not initialised. Call createAuthStore first.');
	}
	return storeInstance;
}
