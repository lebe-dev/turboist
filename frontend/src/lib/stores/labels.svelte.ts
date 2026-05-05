import { labels as labelsApi } from '../api/endpoints/labels';
import { getApiClient } from '../api/client';
import type { Label } from '../api/types';

class LabelsStore {
	items = $state<Label[]>([]);
	loaded = $state<boolean>(false);

	favourites = $derived(this.items.filter((l) => l.isFavourite));
	rest = $derived(this.items.filter((l) => !l.isFavourite));

	async load(): Promise<Label[]> {
		const page = await labelsApi.list(getApiClient(), { limit: 500 });
		this.items = page.items;
		this.loaded = true;
		return page.items;
	}

	upsert(label: Label): void {
		const i = this.items.findIndex((l) => l.id === label.id);
		if (i >= 0) this.items[i] = label;
		else this.items = [...this.items, label];
	}

	remove(id: number): void {
		this.items = this.items.filter((l) => l.id !== id);
	}

	clear(): void {
		this.items = [];
		this.loaded = false;
	}
}

export const labelsStore = new LabelsStore();
