import type { ApiClient } from '../client';
import type { AppSettings, AutoLabelRule } from '../types';

export const appSettings = {
	get(client: ApiClient): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings');
	},

	setAutoLabels(client: ApiClient, rules: AutoLabelRule[]): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/auto-labels', {
			method: 'PUT',
			body: { autoLabels: rules }
		});
	}
};
