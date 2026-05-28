import type { ApiClient } from '../client';
import type {
	AuthLoginResponse,
	AuthLoginSuccessResponse,
	AuthRefreshResponse,
	ClientKind,
	User
} from '../types';

export interface AuthCredentials {
	username: string;
	password: string;
	clientKind: ClientKind;
}

export const auth = {
	setup(client: ApiClient, credentials: AuthCredentials): Promise<AuthLoginSuccessResponse> {
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

	loginOtp(
		client: ApiClient,
		params: { ticket: string; code: string }
	): Promise<AuthLoginSuccessResponse> {
		return client.fetch('/auth/login/otp', {
			method: 'POST',
			body: params,
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

	logoutOthers(client: ApiClient): Promise<void> {
		return client.fetch('/auth/logout-others', {
			method: 'POST',
			skipRefresh: true,
			credentials: 'include'
		});
	},

	me(client: ApiClient): Promise<{ user: User }> {
		return client.fetch('/auth/me');
	}
};
