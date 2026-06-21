import type { ApiClient } from '../client';
import type { InstantiateTemplateResult, Page, TaskTemplate, TaskTemplateInput } from '../types';

export const templates = {
	list(client: ApiClient): Promise<Page<TaskTemplate>> {
		return client.fetch('/api/v1/task-templates');
	},

	get(client: ApiClient, id: number): Promise<TaskTemplate> {
		return client.fetch(`/api/v1/task-templates/${id}`);
	},

	create(client: ApiClient, input: TaskTemplateInput): Promise<TaskTemplate> {
		return client.fetch('/api/v1/task-templates', { method: 'POST', body: input });
	},

	update(client: ApiClient, id: number, input: TaskTemplateInput): Promise<TaskTemplate> {
		return client.fetch(`/api/v1/task-templates/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/task-templates/${id}`, { method: 'DELETE' });
	},

	instantiate(client: ApiClient, id: number, projectId: number): Promise<InstantiateTemplateResult> {
		return client.fetch(`/api/v1/task-templates/${id}/instantiate`, {
			method: 'POST',
			body: { projectId }
		});
	}
};
