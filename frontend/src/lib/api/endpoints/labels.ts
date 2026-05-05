import type { ApiClient } from '../client';
import type { Label, LabelInput, ListQuery, Page, Project, Task, TasksQuery } from '../types';

export interface LabelsListQuery extends ListQuery {
	q?: string;
}

export const labels = {
	list(client: ApiClient, query: LabelsListQuery = {}): Promise<Page<Label>> {
		return client.fetch('/api/v1/labels', { query });
	},

	get(client: ApiClient, id: number): Promise<Label> {
		return client.fetch(`/api/v1/labels/${id}`);
	},

	create(client: ApiClient, input: LabelInput): Promise<Label> {
		return client.fetch('/api/v1/labels', { method: 'POST', body: input });
	},

	update(client: ApiClient, id: number, input: LabelInput): Promise<Label> {
		return client.fetch(`/api/v1/labels/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/labels/${id}`, { method: 'DELETE' });
	},

	listTasks(client: ApiClient, id: number, query: TasksQuery = {}): Promise<Page<Task>> {
		return client.fetch(`/api/v1/labels/${id}/tasks`, { query });
	},

	listProjects(client: ApiClient, id: number, query: ListQuery = {}): Promise<Page<Project>> {
		return client.fetch(`/api/v1/labels/${id}/projects`, { query });
	}
};
