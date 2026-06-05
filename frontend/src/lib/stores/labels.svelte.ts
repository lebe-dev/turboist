import { labels as labelsApi } from '../api/endpoints/labels';
import { getApiClient } from '../api/client';
import type { Label } from '../api/types';
import { dropTombstones, isLive } from '../utils/tombstone';

class LabelsStore {
	items = $state<Label[]>([]);
	loaded = $state<boolean>(false);

	favourites = $derived(this.items.filter((l) => l.isFavourite));
	rest = $derived(this.items.filter((l) => !l.isFavourite));

	async load(): Promise<Label[]> {
		const page = await labelsApi.list(getApiClient(), { limit: 500 });
		const live = dropTombstones(page.items);
		this.items = live;
		this.loaded = true;
		return live;
	}

	setItems(items: Label[]): void {
		this.items = dropTombstones(items);
		this.loaded = true;
	}

	upsert(label: Label): void {
		if (!isLive(label)) {
			this.remove(label.id);
			return;
		}
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
