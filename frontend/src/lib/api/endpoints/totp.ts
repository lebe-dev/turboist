import type { ApiClient } from '../client';
import type { TOTPConfirmResponse, TOTPSetupResponse } from '../types';

export const totp = {
	setup(client: ApiClient): Promise<TOTPSetupResponse> {
		return client.fetch('/auth/totp/setup', { method: 'POST' });
	},

	confirm(client: ApiClient, code: string): Promise<TOTPConfirmResponse> {
		return client.fetch('/auth/totp/confirm', { method: 'POST', body: { code } });
	},

	disable(client: ApiClient, code: string): Promise<void> {
		return client.fetch('/auth/totp/disable', { method: 'POST', body: { code } });
	}
};
