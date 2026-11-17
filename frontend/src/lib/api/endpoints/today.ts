import type { ApiClient } from '../client';
import type { TodayLockResponse, TodayLockState } from '../types';

export const today = {
	/** The running lock and the projects it still lets work for today into. */
	lockState(client: ApiClient): Promise<TodayLockState> {
		return client.fetch('/api/v1/today/lock');
	},
	/** Turns on the Today focus lock until the next midnight (server timezone). */
	lock(client: ApiClient): Promise<TodayLockResponse> {
		return client.fetch('/api/v1/today/lock', { method: 'POST' });
	}
};
