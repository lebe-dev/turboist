import type { ApiClient } from '../client';
import type { UserState } from '../types';

export const state = {
	patch(client: ApiClient, patch: UserState): Promise<UserState> {
		return client.fetch('/api/v1/state', { method: 'PATCH', body: patch });
	}
};
