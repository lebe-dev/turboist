import type { ApiClient, NotModified } from '../client';
import type { ConfigResponse } from '../types';

export const config = {
	get(client: ApiClient): Promise<ConfigResponse> {
		return client.fetch('/api/v1/config');
	},

	/**
	 * Conditional variant used for mid-session resyncs. Resolves to `NOT_MODIFIED`
	 * when the workspace is byte-identical to what we last received, which lets
	 * the caller skip parsing the payload and re-fanning it across ten stores.
	 *
	 * Only safe when the caller already holds a config to fall back on — see
	 * `configStore.refresh()`.
	 */
	getIfChanged(client: ApiClient): Promise<ConfigResponse | NotModified> {
		return client.fetch('/api/v1/config', { conditional: true });
	}
};
