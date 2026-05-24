import type { ApiClient } from '../client';
import type { Session } from '../types';

export const sessions = {
	list(client: ApiClient): Promise<Session[]> {
		return client.fetch('/api/v1/sessions');
	},

	revoke(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/sessions/${id}`, { method: 'DELETE' });
	}
};
