import type { ApiClient } from '../client';
import type { UserSettings } from '../types';

export const settings = {
	patch(client: ApiClient, patch: Partial<UserSettings>): Promise<UserSettings> {
		return client.fetch('/api/v1/settings', { method: 'PATCH', body: patch });
	}
};
