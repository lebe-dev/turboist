import type { ApiClient } from '../client';
import type {
	CalendarEvent,
	CalendarEventsResponse,
	CalendarSettingsResponse,
	CalendarSource
} from '../types';

export const calendars = {
	get(client: ApiClient): Promise<CalendarSettingsResponse> {
		return client.fetch('/api/v1/calendars/');
	},

	setEnabled(client: ApiClient, enabled: boolean): Promise<CalendarSettingsResponse> {
		return client.fetch('/api/v1/calendars/settings', {
			method: 'PATCH',
			body: { enabled }
		});
	},

	googleStart(client: ApiClient): Promise<{ url: string }> {
		return client.fetch('/api/v1/calendars/google/start');
	},

	googleSync(client: ApiClient): Promise<CalendarSettingsResponse> {
		return client.fetch('/api/v1/calendars/google/sync', { method: 'POST' });
	},

	setSourceSelected(client: ApiClient, id: number, selected: boolean): Promise<CalendarSource> {
		return client.fetch(`/api/v1/calendars/sources/${id}`, {
			method: 'PATCH',
			body: { selected }
		});
	},

	deleteAccount(client: ApiClient, id: number): Promise<void> {
		return client.fetch(`/api/v1/calendars/accounts/${id}`, { method: 'DELETE' });
	},

	events(client: ApiClient, start: string, end: string): Promise<CalendarEvent[]> {
		return client
			.fetch<CalendarEventsResponse>('/api/v1/calendars/events', {
				query: { start, end }
			})
			.then((res) => res.items ?? []);
	}
};
