import type { ApiClient } from '../client';
import type { TodayLockResponse } from '../types';

export const today = {
	/** Turns on the Today focus lock until the next midnight (server timezone). */
	lock(client: ApiClient): Promise<TodayLockResponse> {
		return client.fetch('/api/v1/today/lock', { method: 'POST' });
	}
};
