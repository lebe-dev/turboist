import type { ApiClient } from '../client';
import type { ClientKind, Passkey, PasskeyCeremony, AuthLoginSuccessResponse } from '../types';

export const passkeys = {
	list(client: ApiClient): Promise<Passkey[]> {
		return client.fetch('/api/v1/passkeys');
	},

	registerBegin(client: ApiClient): Promise<PasskeyCeremony> {
		return client.fetch('/api/v1/passkeys/register/begin', { method: 'POST' });
	},

	registerFinish(
		client: ApiClient,
		params: { ceremonyId: string; name: string; credential: Record<string, unknown> }
	): Promise<Passkey> {
		return client.fetch('/api/v1/passkeys/register/finish', { method: 'POST', body: params });
	},

	rename(client: ApiClient, id: number, name: string): Promise<Passkey> {
		return client.fetch(`/api/v1/passkeys/${id}`, { method: 'PATCH', body: { name } });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/passkeys/${id}`, { method: 'DELETE' });
	},

	// The two login halves run before there is any session, hence the same
	// skipAuth/skipRefresh/credentials shape the password endpoints use.
	loginBegin(client: ApiClient, clientKind: ClientKind): Promise<PasskeyCeremony> {
		return client.fetch('/auth/passkey/login/begin', {
			method: 'POST',
			body: { clientKind },
			skipAuth: true,
			skipRefresh: true,
			credentials: 'include'
		});
	},

	loginFinish(
		client: ApiClient,
		params: { ceremonyId: string; clientKind: ClientKind; credential: Record<string, unknown> }
	): Promise<AuthLoginSuccessResponse> {
		return client.fetch('/auth/passkey/login/finish', {
			method: 'POST',
			body: params,
			skipAuth: true,
			skipRefresh: true,
			credentials: 'include'
		});
	}
};
