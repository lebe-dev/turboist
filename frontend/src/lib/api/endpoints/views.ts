import type { ApiClient } from '../client';
import type {
	PlanStatsResponse,
	SearchQuery,
	SearchResponse,
	Task,
	TodayBundle,
	ViewList,
	ViewPageQuery,
	ViewQuery
} from '../types';

export const views = {
	// today returns the bundle the Today page needs (today + overdue +
	// completedToday) in a single request. Backend handler combines them
	// behind `/api/v1/tasks/today`.
	today(client: ApiClient, query: ViewPageQuery = {}): Promise<TodayBundle> {
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

	completed(
		client: ApiClient,
		query: ViewPageQuery & { days?: number } = {}
	): Promise<ViewList<Task>> {
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
