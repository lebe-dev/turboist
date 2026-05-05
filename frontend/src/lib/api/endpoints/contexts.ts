import type { ApiClient } from '../client';
import type {
	Context,
	ContextInput,
	ListQuery,
	Page,
	Project,
	ProjectInput,
	Task,
	TaskInput,
	TasksQuery
} from '../types';

export const contexts = {
	list(client: ApiClient, query: ListQuery = {}): Promise<Page<Context>> {
		return client.fetch('/api/v1/contexts', { query });
	},

	get(client: ApiClient, id: number): Promise<Context> {
		return client.fetch(`/api/v1/contexts/${id}`);
	},

	create(client: ApiClient, input: ContextInput): Promise<Context> {
		return client.fetch('/api/v1/contexts', { method: 'POST', body: input });
	},

	update(client: ApiClient, id: number, input: ContextInput): Promise<Context> {
		return client.fetch(`/api/v1/contexts/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/contexts/${id}`, { method: 'DELETE' });
	},

	listProjects(client: ApiClient, id: number, query: ListQuery = {}): Promise<Page<Project>> {
		return client.fetch(`/api/v1/contexts/${id}/projects`, { query });
	},

	createProject(client: ApiClient, id: number, input: ProjectInput): Promise<Project> {
		return client.fetch(`/api/v1/contexts/${id}/projects`, { method: 'POST', body: input });
	},

	listTasks(client: ApiClient, id: number, query: TasksQuery = {}): Promise<Page<Task>> {
		return client.fetch(`/api/v1/contexts/${id}/tasks`, { query });
	},

	createTask(client: ApiClient, id: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/contexts/${id}/tasks`, { method: 'POST', body: input });
	}
};
