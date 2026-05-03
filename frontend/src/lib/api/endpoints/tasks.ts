import type { ApiClient } from '../client';
import type {
	BulkResult,
	InboxResponse,
	Task,
	TaskInput,
	TaskMoveInput,
	TaskPlanInput,
	TroikiCategory
} from '../types';

export const tasks = {
	get(client: ApiClient, id: number): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${id}`);
	},

	update(client: ApiClient, id: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${id}`, { method: 'PATCH', body: input });
	},

	remove(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/tasks/${id}`, { method: 'DELETE' });
	},

	complete: (client: ApiClient, id: number) => action(client, id, 'complete'),
	uncomplete: (client: ApiClient, id: number) => action(client, id, 'uncomplete'),
	cancel: (client: ApiClient, id: number) => action(client, id, 'cancel'),
	pin: (client: ApiClient, id: number) => action(client, id, 'pin'),
	unpin: (client: ApiClient, id: number) => action(client, id, 'unpin'),
	duplicate: (client: ApiClient, id: number) => action(client, id, 'duplicate'),

	move(client: ApiClient, id: number, input: TaskMoveInput): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${id}/move`, { method: 'POST', body: input });
	},

	plan(client: ApiClient, id: number, input: TaskPlanInput): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${id}/plan`, { method: 'POST', body: input });
	},

	setTroikiCategory(
		client: ApiClient,
		id: number,
		category: TroikiCategory | null
	): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${id}/troiki`, {
			method: 'POST',
			body: { category }
		});
	},

	createSubtask(client: ApiClient, parentId: number, input: TaskInput): Promise<Task> {
		return client.fetch(`/api/v1/tasks/${parentId}/subtasks`, {
			method: 'POST',
			body: input
		});
	},

	bulkComplete(client: ApiClient, ids: number[]): Promise<BulkResult> {
		return client.fetch('/api/v1/tasks/bulk/complete', {
			method: 'POST',
			body: { ids }
		});
	},

	bulkMove(client: ApiClient, ids: number[], target: TaskMoveInput): Promise<BulkResult> {
		return client.fetch('/api/v1/tasks/bulk/move', {
			method: 'POST',
			body: { ids, ...target }
		});
	},

	inbox(client: ApiClient): Promise<InboxResponse> {
		return client.fetch('/api/v1/inbox');
	},

	createInbox(client: ApiClient, input: TaskInput): Promise<Task> {
		return client.fetch('/api/v1/inbox/tasks', { method: 'POST', body: input });
	}
};

function action(client: ApiClient, id: number, name: string): Promise<Task> {
	return client.fetch(`/api/v1/tasks/${id}/${name}`, { method: 'POST' });
}
