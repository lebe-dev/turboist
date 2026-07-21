import { ApiClient, setApiClient, type OfflineBridge } from '../api/client';
import { ApiError } from '../api/errors';
import { createOfflineBridge } from '../offline';
import { auth, type AuthCredentials } from '../api/endpoints/auth';
import type { User, ClientKind, AuthLoginSuccessResponse } from '../api/types';
import { addApiLogEntry } from '../stores/apiLog.svelte';
import { clientOrigin } from '../realtime/origin';
import { onSelfMutation } from '../realtime/selfRefresh';
import { isNativePlatform, resolveClientKind } from '../native/platform';
import { nativeRefreshTokenStore, type RefreshTokenStore } from '../native/secureToken';
import { getServerUrl } from '../native/serverUrl';

export type AuthStatus = 'loading' | 'guest' | 'authenticated';

// Outcome of the boot-time refresh attempt (§4.9). Distinguishing a network
// failure from an auth rejection is what lets an offline boot render from cache
// instead of bouncing to /login.
//  - 'ok'       — refresh succeeded, an access token is set;
//  - 'network'  — the server was unreachable (ApiError.status === 0): offline
//                 session, NOT a rejection — the stored token is left intact;
//  - 'rejected' — the server responded and rejected the token (401/403), or
//                 (native) there is no stored token at all → plain guest.
type RefreshOutcome = 'ok' | 'network' | 'rejected';

export interface AuthStoreOptions {
	baseUrl?: string;
	fetchImpl?: typeof fetch;
	// Native only. Web leaves both unset: clientKind defaults to 'web' and the
	// refresh token stays in the HttpOnly cookie (no tokenStore).
	clientKind?: ClientKind;
	tokenStore?: RefreshTokenStore | null;
	// Offline read/queue bridge (§4.10). Undefined → purely-online ApiClient
	// (unchanged behaviour). The `createAuthStore` factory assembles the real
	// bridge; `new AuthStore(...)` in unit tests leaves it unset and hermetic.
	offline?: OfflineBridge;
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
	// True when the app booted without reaching the server (§4.9): the session
	// could not be validated, so we render from the read-through cache instead of
	// redirecting to /login. Cleared the moment we get a definitive answer — a
	// successful login, a real refresh rejection (onRefreshFailure) or logout.
	offlineSession = $state<boolean>(false);

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
				// A hard rejection ends any offline session: fall through to /login.
				this.offlineSession = false;
			},
			onLog: (entry) => addApiLogEntry(entry),
			clientOrigin,
			onMutation: (path) => onSelfMutation(path),
			getRefreshToken: this.tokenStore ? () => this.tokenStore!.get() : undefined,
			setRefreshToken: this.tokenStore ? (t) => this.tokenStore!.set(t) : undefined,
			offline: options.offline
		});
		setApiClient(this.client);
	}

	async bootstrap(): Promise<BootstrapResult> {
		this.status = 'loading';
		this.offlineSession = false;

		const outcome = await this.tryRefresh();

		if (outcome === 'network') {
			// Boot without network (§4.9): the server was unreachable, so the
			// session cannot be validated. Do NOT bounce to /login — enter an
			// offline session and let the (app) shell render from the read-through
			// cache. `status` stays 'authenticated' so the existing render/load
			// gates work unchanged; the first request after reconnect either
			// refreshes silently or, on a truly dead token, 401s →
			// onRefreshFailure → guest → /login.
			this.offlineSession = true;
			this.status = 'authenticated';
			return { setupRequired: false, authenticated: false };
		}

		if (outcome === 'rejected') {
			// Refresh failed with a server response (no/expired token): probe
			// /api/v1/config without auth. SetupCheckMiddleware runs before the auth
			// middleware on the /api/v1 group, so an un-set-up instance returns 503
			// setup_required; a set-up instance returns 401 (plain guest).
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

	private async tryRefresh(): Promise<RefreshOutcome> {
		let stored: string | undefined;
		if (this.tokenStore) {
			const rt = await this.tokenStore.get();
			// Native and logged out: no stored token → plain guest, no network call
			// (and therefore no offline session — there is nothing to render).
			if (!rt) return 'rejected';
			stored = rt;
		}
		try {
			const res = await auth.refresh(this.client, stored);
			this.accessToken = res.access;
			if (this.tokenStore && res.refresh) await this.tokenStore.set(res.refresh);
			return 'ok';
		} catch (err) {
			// Network error (ApiError.status === 0): the server is unreachable, not
			// rejecting us. Keep the stored token — it may still be valid — and let
			// bootstrap enter an offline session (§4.9).
			if (err instanceof ApiError && err.status === 0) {
				return 'network';
			}
			if (
				this.tokenStore &&
				err instanceof ApiError &&
				(err.status === 401 || err.status === 403)
			) {
				// Dead token — clear it so we don't retry the same one every launch.
				await this.tokenStore.set(null);
			}
			return 'rejected';
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
		this.offlineSession = false;
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
		this.offlineSession = false;
		if (this.tokenStore) await this.tokenStore.set(null);
	}
}

let storeInstance: AuthStore | null = null;

export function createAuthStore(options: AuthStoreOptions = {}): AuthStore {
	// The factory is the ONLY place the platform is resolved, so unit tests that
	// construct `new AuthStore({...})` directly stay hermetic — web defaults,
	// never touching the Capacitor bridge.
	const baseUrl = options.baseUrl ?? (isNativePlatform() ? getServerUrl() : undefined);
	const resolved: AuthStoreOptions = {
		...options,
		clientKind: options.clientKind ?? resolveClientKind(),
		tokenStore: options.tokenStore ?? (isNativePlatform() ? nativeRefreshTokenStore : null),
		baseUrl,
		// Assemble the offline bridge here (the one place the platform is resolved)
		// so it is namespaced to the resolved server and `new AuthStore(...)` in
		// tests stays purely online (§4.10). Degrades to a no-op without IndexedDB.
		offline: options.offline ?? createOfflineBridge({ serverUrl: baseUrl ?? '' })
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
