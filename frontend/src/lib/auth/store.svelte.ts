import { ApiClient, setApiClient } from '../api/client';
import { ApiError } from '../api/errors';
import { auth, type AuthCredentials } from '../api/endpoints/auth';
import type { User } from '../api/types';

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
		} catch {
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
		}

		const refreshed = await this.tryRefresh();
		if (!refreshed) {
			this.status = 'guest';
			return { setupRequired: false, authenticated: false };
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

	async login(credentials: Omit<AuthCredentials, 'clientKind'>): Promise<void> {
		const res = await auth.login(this.client, { ...credentials, clientKind: 'web' });
		this.accessToken = res.access;
		this.user = res.user;
		this.status = 'authenticated';
		this.setupRequired = false;
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
