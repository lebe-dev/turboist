import type { ApiClient } from '../client';
import type {
	SearchQuery,
	SearchResponse,
	SidebarStatsResponse,
	Task,
	TodayBundle,
	ViewList,
	ViewPageQuery,
	ViewQuery,
	WeekSummaryResponse
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

	// sidebarStats bundles plan counters, the inbox badge and the pinned list in
	// one request. The originating client refetches this once after its own
	// mutation (its SSE echo is suppressed) instead of the three separate
	// /stats/plan + /inbox + /tasks/pinned round-trips.
	sidebarStats(client: ApiClient): Promise<SidebarStatsResponse> {
		return client.fetch('/api/v1/stats/sidebar');
	},

	// weekSummary returns the current week's review: completed tasks plus the
	// headline counters (completed / still planned / overdue). Breakdowns are
	// computed client-side from `completed`.
	weekSummary(client: ApiClient): Promise<WeekSummaryResponse> {
		return client.fetch('/api/v1/stats/week-summary');
	},

	search(client: ApiClient, query: SearchQuery): Promise<SearchResponse> {
		return client.fetch('/api/v1/search', { query });
	}
};
