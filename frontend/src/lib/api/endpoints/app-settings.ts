import type { ApiClient } from '../client';
import type { AppSettings, AutoLabelRule, ProjectSuggestionRule } from '../types';

export const appSettings = {
	get(client: ApiClient): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings');
	},

	setAutoLabels(client: ApiClient, rules: AutoLabelRule[]): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/auto-labels', {
			method: 'PUT',
			body: { autoLabels: rules }
		});
	},

	setProjectSuggestions(
		client: ApiClient,
		rules: ProjectSuggestionRule[]
	): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/project-suggestions', {
			method: 'PUT',
			body: { projectSuggestions: rules }
		});
	}
};
