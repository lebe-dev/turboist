import type { ApiClient } from '../client';
import type { APIToken, APITokenWithSecret } from '../types';

export const apiTokens = {
	list(client: ApiClient): Promise<APIToken[]> {
		return client.fetch('/api/v1/api-tokens');
	},

	create(client: ApiClient, name: string): Promise<APITokenWithSecret> {
		return client.fetch('/api/v1/api-tokens', { method: 'POST', body: { name } });
	},

	delete(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/api-tokens/${id}`, { method: 'DELETE' });
	}
};
