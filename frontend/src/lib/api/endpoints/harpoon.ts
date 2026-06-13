import type { ApiClient } from '../client';
import type { HarpoonKind, HarpoonState } from '../types';

export const harpoon = {
	get(client: ApiClient): Promise<HarpoonState> {
		return client.fetch('/api/v1/harpoon');
	},

	attach(client: ApiClient, kind: HarpoonKind, id: number): Promise<HarpoonState> {
		return client.fetch('/api/v1/harpoon/attach', { method: 'POST', body: { kind, id } });
	},

	detach(client: ApiClient, kind: HarpoonKind, id: number): Promise<HarpoonState> {
		return client.fetch('/api/v1/harpoon/detach', { method: 'POST', body: { kind, id } });
	}
};
