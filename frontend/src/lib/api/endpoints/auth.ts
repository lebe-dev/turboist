import type { ApiClient } from '../client';
import type {
	AuthLoginResponse,
	AuthRefreshResponse,
	AuthSetupRequiredResponse,
	ClientKind,
	User
} from '../types';

export interface AuthCredentials {
	username: string;
	password: string;
	clientKind: ClientKind;
}

export const auth = {
	setupRequired(client: ApiClient): Promise<AuthSetupRequiredResponse> {
		return client.fetch('/auth/setup-required', { skipAuth: true, skipRefresh: true });
	},

	setup(client: ApiClient, credentials: AuthCredentials): Promise<AuthLoginResponse> {
		return client.fetch('/auth/setup', {
			method: 'POST',
			body: credentials,
			skipAuth: true,
			skipRefresh: true,
			credentials: 'include'
		});
	},

	login(client: ApiClient, credentials: AuthCredentials): Promise<AuthLoginResponse> {
		return client.fetch('/auth/login', {
			method: 'POST',
			body: credentials,
			skipAuth: true,
			skipRefresh: true,
			credentials: 'include'
		});
	},

	refresh(client: ApiClient): Promise<AuthRefreshResponse> {
		return client.fetch('/auth/refresh', {
			method: 'POST',
			skipAuth: true,
			skipRefresh: true,
			credentials: 'include'
		});
	},

	logout(client: ApiClient): Promise<void> {
		return client.fetch('/auth/logout', {
			method: 'POST',
			skipRefresh: true,
			credentials: 'include'
		});
	},

	logoutAll(client: ApiClient): Promise<void> {
		return client.fetch('/auth/logout-all', {
			method: 'POST',
			skipRefresh: true,
			credentials: 'include'
		});
	},

	me(client: ApiClient): Promise<{ user: User }> {
		return client.fetch('/auth/me');
	}
};
