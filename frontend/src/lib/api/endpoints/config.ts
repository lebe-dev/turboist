import type { ApiClient } from '../client';
import type { ConfigResponse } from '../types';

export const config = {
	get(client: ApiClient): Promise<ConfigResponse> {
		return client.fetch('/api/v1/config');
	}
};
