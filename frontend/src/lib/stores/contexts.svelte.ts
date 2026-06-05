import { contexts as contextsApi } from '../api/endpoints/contexts';
import { getApiClient } from '../api/client';
import type { Context } from '../api/types';
import { dropTombstones, isLive } from '../utils/tombstone';

class ContextsStore {
	items = $state<Context[]>([]);
	loaded = $state<boolean>(false);

	async load(): Promise<Context[]> {
		const page = await contextsApi.list(getApiClient(), { limit: 200 });
		const live = dropTombstones(page.items);
		this.items = live;
		this.loaded = true;
		return live;
	}

	setItems(items: Context[]): void {
		this.items = dropTombstones(items);
		this.loaded = true;
	}

	upsert(ctx: Context): void {
		if (!isLive(ctx)) {
			this.remove(ctx.id);
			return;
		}
		const i = this.items.findIndex((c) => c.id === ctx.id);
		if (i >= 0) this.items[i] = ctx;
		else this.items = [...this.items, ctx];
	}

	remove(id: number): void {
		this.items = this.items.filter((c) => c.id !== id);
	}

	clear(): void {
		this.items = [];
		this.loaded = false;
	}
}

export const contextsStore = new ContextsStore();
