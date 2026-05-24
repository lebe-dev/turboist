import type { ApiClient } from '../client';

export interface EventsTicket {
	ticket: string;
	expiresIn: number;
}

export const events = {
	issueTicket(client: ApiClient): Promise<EventsTicket> {
		return client.fetch<EventsTicket>('/api/v1/events/ticket', { method: 'POST' });
	}
};
