import { ApiClient, setApiClient } from '../api/client';
import { ApiError } from '../api/errors';
import { auth, type AuthCredentials } from '../api/endpoints/auth';
import type { User } from '../api/types';
import { addApiLogEntry } from '../stores/apiLog.svelte';

export type AuthStatus = 'loading' | 'guest' | 'authenticated';

export interface AuthStoreOptions {
	baseUrl?: string;
	fetchImpl?: typeof fetch;
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

	constructor(options: AuthStoreOptions = {}) {
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
			onLog: (entry) => addApiLogEntry(entry)
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
			const res = await auth.refresh(this.client);
			this.accessToken = res.access;
			return true;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				return false;
			}
			return false;
		}
	}

	async login(
		credentials: Omit<AuthCredentials, 'clientKind'>
	): Promise<{ otpRequired: boolean }> {
		const res = await auth.login(this.client, { ...credentials, clientKind: 'web' });
		if ('otpRequired' in res) {
			this.otpTicket = res.ticket;
			this.awaitingOtp = true;
			return { otpRequired: true };
		}
		this.otpTicket = null;
		this.awaitingOtp = false;
		this.accessToken = res.access;
		this.user = res.user;
		this.status = 'authenticated';
		this.setupRequired = false;
		return { otpRequired: false };
	}

	async verifyOtp(code: string): Promise<void> {
		if (!this.otpTicket) {
			throw new Error('No OTP challenge in progress');
		}
		const res = await auth.loginOtp(this.client, { ticket: this.otpTicket, code });
		this.otpTicket = null;
		this.awaitingOtp = false;
		this.accessToken = res.access;
		this.user = res.user;
		this.status = 'authenticated';
		this.setupRequired = false;
	}

	cancelOtp(): void {
		this.otpTicket = null;
		this.awaitingOtp = false;
	}

	async setup(credentials: Omit<AuthCredentials, 'clientKind'>): Promise<void> {
		const res = await auth.setup(this.client, { ...credentials, clientKind: 'web' });
		this.accessToken = res.access;
		this.user = res.user;
		this.status = 'authenticated';
		this.setupRequired = false;
	}

	async logout(): Promise<void> {
		try {
			await auth.logout(this.client);
		} catch {
			// best-effort; clear local state regardless
		}
		this.clear();
	}

	async logoutAll(): Promise<void> {
		try {
			await auth.logoutAll(this.client);
		} catch {
			// best-effort
		}
		this.clear();
	}

	private clear(): void {
		this.user = null;
		this.accessToken = null;
		this.status = 'guest';
		this.otpTicket = null;
		this.awaitingOtp = false;
	}
}

let storeInstance: AuthStore | null = null;

export function createAuthStore(options: AuthStoreOptions = {}): AuthStore {
	storeInstance = new AuthStore(options);
	return storeInstance;
}

export function getAuthStore(): AuthStore {
	if (!storeInstance) {
		throw new Error('AuthStore is not initialised. Call createAuthStore first.');
	}
	return storeInstance;
}
