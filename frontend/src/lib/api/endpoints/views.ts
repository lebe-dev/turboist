import type { ApiClient } from '../client';
import type { SearchQuery, SearchResponse, Task, ViewList, ViewPageQuery, ViewQuery } from '../types';

export const views = {
	today(client: ApiClient, query: ViewPageQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/today', { query });
	},

	tomorrow(client: ApiClient, query: ViewPageQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/tomorrow', { query });
	},

	overdue(client: ApiClient, query: ViewPageQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/overdue', { query });
	},

	week(client: ApiClient, query: ViewQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/week', { query });
	},

	backlog(client: ApiClient, query: ViewQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/backlog', { query });
	},

	search(client: ApiClient, query: SearchQuery): Promise<SearchResponse> {
		return client.fetch('/api/v1/search', { query });
	}
};
