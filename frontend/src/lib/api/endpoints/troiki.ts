import type { ApiClient } from '../client';
import type { TroikiViewResponse } from '../types';

export const troiki = {
	view(client: ApiClient): Promise<TroikiViewResponse> {
		return client.fetch('/api/v1/troiki');
	},
	start(client: ApiClient): Promise<TroikiViewResponse> {
		return client.fetch('/api/v1/troiki/start', { method: 'POST' });
	}
};
