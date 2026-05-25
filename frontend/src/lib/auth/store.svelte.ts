import { ApiClient, setApiClient } from '../api/client';
import { ApiError } from '../api/errors';
import { auth, type AuthCredentials } from '../api/endpoints/auth';
import type { User } from '../api/types';
import { NOOP_OFFLINE_AUTH_ADAPTER, type OfflineAuthAdapter } from '../offline/auth';

export type AuthStatus = 'loading' | 'guest' | 'authenticated';

export interface AuthStoreOptions {
	baseUrl?: string;
	fetchImpl?: typeof fetch;
	offline?: OfflineAuthAdapter;
}

interface BootstrapResult {
	setupRequired: boolean;
	authenticated: boolean;
}

type RefreshOutcome = 'ok' | 'rejected' | 'offline';

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
	private readonly offline: OfflineAuthAdapter;

	constructor(options: AuthStoreOptions = {}) {
		this.offline = options.offline ?? NOOP_OFFLINE_AUTH_ADAPTER;
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
				// Real auth rejection mid-session: drop cached offline data.
				void this.offline.clear();
			}
		});
		setApiClient(this.client);
	}

	async bootstrap(): Promise<BootstrapResult> {
		this.status = 'loading';
		try {
			const setup = await auth.setupRequired(this.client);
			this.setupRequired = setup.required;
			if (setup.required) {
				this.status = 'guest';
				return { setupRequired: true, authenticated: false };
			}
		} catch (err) {
			if (await this.tryAuthenticateOffline(err)) {
				return { setupRequired: false, authenticated: true };
			}
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}

		const refreshed = await this.tryRefresh();
		if (refreshed === 'rejected') {
			await this.offline.clear();
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}
		if (refreshed === 'offline') {
			if (await this.tryAuthenticateOffline(null)) {
				return { setupRequired: false, authenticated: true };
			}
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}

		try {
			const me = await auth.me(this.client);
			this.user = me.user;
			this.status = 'authenticated';
			await this.offline.saveUser(me.user);
			await this.offline.onAuthenticatedRefresh();
			return { setupRequired: false, authenticated: true };
		} catch (err) {
			if (await this.tryAuthenticateOffline(err)) {
				return { setupRequired: false, authenticated: true };
			}
			this.accessToken = null;
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}
	}

	private async tryAuthenticateOffline(err: unknown): Promise<boolean> {
		if (err !== null && !isNetworkError(err)) return false;
		const cached = await this.offline.loadUser();
		if (!cached) return false;
		if (!(await this.offline.hasData())) return false;
		this.user = cached.user;
		this.accessToken = null;
		this.status = 'authenticated';
		return true;
	}

	private async tryRefresh(): Promise<RefreshOutcome> {
		try {
			const res = await auth.refresh(this.client);
			this.accessToken = res.access;
			return 'ok';
		} catch (err) {
			if (isNetworkError(err)) return 'offline';
			return 'rejected';
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
		await this.offline.saveUser(res.user);
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
		await this.offline.saveUser(res.user);
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
		await this.offline.saveUser(res.user);
	}

	async logout(): Promise<void> {
		try {
			await auth.logout(this.client);
		} catch {
			// best-effort; clear local state regardless
		}
		await this.offline.clear();
		this.clear();
	}

	async logoutAll(): Promise<void> {
		try {
			await auth.logoutAll(this.client);
		} catch {
			// best-effort
		}
		await this.offline.clear();
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

const isNetworkError = (err: unknown): boolean =>
	err instanceof ApiError && err.status === 0;

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
