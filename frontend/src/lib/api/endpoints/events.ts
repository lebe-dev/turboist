import type { ApiClient } from '../client';

export interface EventsTicket {
	ticket: string;
	expiresIn: number;
}

export const events = {
	// origin binds the stream to this client so the backend skips echoing the
	// client's own mutations back to it (see lib/realtime/origin.ts).
	issueTicket(client: ApiClient, origin?: string): Promise<EventsTicket> {
		return client.fetch<EventsTicket>('/api/v1/events/ticket', {
			method: 'POST',
			body: origin ? { origin } : undefined
		});
	}
};
