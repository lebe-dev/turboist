import type { ApiClient } from '../client';
import type { ListQuery, Page, ProjectSection, SectionInput, Task, TaskInput, TasksQuery } from '../types';

export const sections = {
	get(client: ApiClient, id: number): Promise<ProjectSection> {
		return client.fetch(`/api/v1/sections/${id}`);
	},

	update(client: ApiClient, id: number, input: SectionInput): Promise<ProjectSection> {
		return client.fetch(`/api/v1/sections/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/sections/${id}`, { method: 'DELETE' });
	},

	listTasks(client: ApiClient, id: number, query: TasksQuery = {}): Promise<Page<Task>> {
		return client.fetch(`/api/v1/sections/${id}/tasks`, { query });
	},

	createTask(client: ApiClient, id: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/sections/${id}/tasks`, { method: 'POST', body: input });
	},

	reorder(client: ApiClient, id: number, position: number, _query: ListQuery = {}): Promise<ProjectSection> {
		return client.fetch(`/api/v1/sections/${id}/reorder`, {
			method: 'POST',
			body: { position }
		});
	}
};
