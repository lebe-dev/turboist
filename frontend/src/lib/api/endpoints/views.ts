import type { ApiClient } from '../client';
import type {
	PlanStatsResponse,
	SearchQuery,
	SearchResponse,
	Task,
	ViewList,
	ViewPageQuery,
	ViewQuery
} from '../types';

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

	completedToday(client: ApiClient, query: ViewPageQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/completed', { query });
	},

	week(client: ApiClient, query: ViewQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/week', { query });
	},

	backlog(client: ApiClient, query: ViewQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/backlog', { query });
	},

	pinned(client: ApiClient, query: ViewQuery = {}): Promise<ViewList<Task>> {
		return client.fetch('/api/v1/tasks/pinned', { query });
	},

	planStats(client: ApiClient): Promise<PlanStatsResponse> {
		return client.fetch('/api/v1/stats/plan');
	},

	search(client: ApiClient, query: SearchQuery): Promise<SearchResponse> {
		return client.fetch('/api/v1/search', { query });
	}
};
