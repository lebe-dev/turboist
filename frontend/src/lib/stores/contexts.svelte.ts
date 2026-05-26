import { contexts as contextsApi } from '../api/endpoints/contexts';
import { getApiClient } from '../api/client';
import type { Context } from '../api/types';
import { getDB } from '../offline/db';

class ContextsStore {
	items = $state<Context[]>([]);
	loaded = $state<boolean>(false);

	async load(): Promise<Context[]> {
		const page = await contextsApi.list(getApiClient(), { limit: 200 });
		this.items = page.items;
		this.loaded = true;
		return page.items;
	}

	async loadCached(): Promise<boolean> {
		const rows = await getDB().table('contexts').toArray();
		const items: Context[] = [];
		for (const row of rows) {
			if (row.deletedAt !== null) continue;
			items.push(row.data as Context);
		}
		if (items.length === 0) return false;
		this.items = items;
		this.loaded = true;
		return true;
	}

	upsert(ctx: Context): void {
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
