import type { ApiClient } from '../client';
import type { InboxProcessingLogEntry, InboxProcessingStatus, Page, Task } from '../types';

export const inboxProcessing = {
	status(client: ApiClient): Promise<InboxProcessingStatus> {
		return client.fetch('/api/v1/inbox/processing');
	},

	run(client: ApiClient): Promise<{ running: boolean }> {
		return client.fetch('/api/v1/inbox/processing/run', { method: 'POST' });
	},

	// `prompt` omitted previews the saved prompt; an empty string previews the
	// built-in default.
	preview(client: ApiClient, prompt?: string): Promise<{ rendered: string }> {
		return client.fetch('/api/v1/inbox/processing/preview', {
			method: 'POST',
			body: prompt === undefined ? {} : { prompt }
		});
	},

	log(client: ApiClient, limit = 50, offset = 0): Promise<Page<InboxProcessingLogEntry>> {
		return client.fetch('/api/v1/inbox/processing/log', { query: { limit, offset } });
	},

	revert(client: ApiClient, id: number): Promise<Task> {
		return client.fetch(`/api/v1/inbox/processing/log/${id}/revert`, { method: 'POST' });
	}
};
