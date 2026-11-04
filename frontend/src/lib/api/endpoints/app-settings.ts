import type { ApiClient } from '../client';
import type {
	AppSettings,
	AutoLabelRule,
	LinkTitlesSettings,
	ProjectSuggestionRule
} from '../types';

export const appSettings = {
	setAutoLabels(client: ApiClient, rules: AutoLabelRule[]): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/auto-labels', {
			method: 'PUT',
			body: { autoLabels: rules }
		});
	},

	setProjectSuggestions(client: ApiClient, rules: ProjectSuggestionRule[]): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/project-suggestions', {
			method: 'PUT',
			body: { projectSuggestions: rules }
		});
	},

	setInboxProcessingPrompt(client: ApiClient, prompt: string): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/inbox-processing', {
			method: 'PUT',
			body: { prompt }
		});
	},

	setInboxProcessingPaused(client: ApiClient, paused: boolean): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/inbox-processing', {
			method: 'PUT',
			body: { paused }
		});
	},

	setLinkTitles(client: ApiClient, patch: Partial<LinkTitlesSettings>): Promise<AppSettings> {
		return client.fetch('/api/v1/app-settings/link-titles', {
			method: 'PUT',
			body: patch
		});
	}
};
