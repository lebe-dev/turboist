import type { ApiClient } from '../client';
import type { ProjectSection, SectionInput, Task, TaskInput } from '../types';

export const sections = {
	update(client: ApiClient, id: number, input: SectionInput): Promise<ProjectSection> {
		return client.fetch(`/api/v1/sections/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/sections/${id}`, { method: 'DELETE' });
	},

	createTask(client: ApiClient, id: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/sections/${id}/tasks`, { method: 'POST', body: input });
	},

	reorder(client: ApiClient, id: number, position: number): Promise<ProjectSection> {
		return client.fetch(`/api/v1/sections/${id}/reorder`, {
			method: 'POST',
			body: { position }
		});
	}
};
