import type { ApiClient } from '../client';
import type {
	ListQuery,
	Page,
	Project,
	ProjectInput,
	ProjectSection,
	ProjectsQuery,
	SectionInput,
	Task,
	TaskInput,
	TasksQuery,
	TroikiCategory
} from '../types';

export const projects = {
	list(client: ApiClient, query: ProjectsQuery = {}): Promise<Page<Project>> {
		return client.fetch('/api/v1/projects', { query });
	},

	get(client: ApiClient, id: number): Promise<Project> {
		return client.fetch(`/api/v1/projects/${id}`);
	},

	update(client: ApiClient, id: number, input: ProjectInput): Promise<Project> {
		return client.fetch(`/api/v1/projects/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/projects/${id}`, { method: 'DELETE' });
	},

	complete: (client: ApiClient, id: number) => action(client, id, 'complete'),
	uncomplete: (client: ApiClient, id: number) => action(client, id, 'uncomplete'),
	cancel: (client: ApiClient, id: number) => action(client, id, 'cancel'),
	archive: (client: ApiClient, id: number) => action(client, id, 'archive'),
	unarchive: (client: ApiClient, id: number) => action(client, id, 'unarchive'),
	pin: (client: ApiClient, id: number) => action(client, id, 'pin'),
	unpin: (client: ApiClient, id: number) => action(client, id, 'unpin'),

	listSections(client: ApiClient, id: number, query: ListQuery = {}): Promise<Page<ProjectSection>> {
		return client.fetch(`/api/v1/projects/${id}/sections`, { query });
	},

	createSection(client: ApiClient, id: number, input: SectionInput): Promise<ProjectSection> {
		return client.fetch(`/api/v1/projects/${id}/sections`, { method: 'POST', body: input });
	},

	listTasks(client: ApiClient, id: number, query: TasksQuery = {}): Promise<Page<Task>> {
		return client.fetch(`/api/v1/projects/${id}/tasks`, { query });
	},

	createTask(client: ApiClient, id: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/projects/${id}/tasks`, { method: 'POST', body: input });
	},

	setTroikiCategory(
		client: ApiClient,
		id: number,
		category: TroikiCategory | null
	): Promise<Project> {
		return client.fetch(`/api/v1/projects/${id}/troiki`, {
			method: 'POST',
			body: { category }
		});
	}
};

function action(client: ApiClient, id: number, name: string): Promise<Project> {
	return client.fetch(`/api/v1/projects/${id}/${name}`, { method: 'POST' });
}
