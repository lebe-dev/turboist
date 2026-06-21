import { templates as templatesApi } from '../api/endpoints/templates';
import { getApiClient } from '../api/client';
import type { TaskTemplate } from '../api/types';

class TemplatesStore {
	items = $state<TaskTemplate[]>([]);
	loaded = $state<boolean>(false);

	async load(): Promise<TaskTemplate[]> {
		const page = await templatesApi.list(getApiClient());
		this.items = page.items;
		this.loaded = true;
		return page.items;
	}

	upsert(template: TaskTemplate): void {
		const i = this.items.findIndex((t) => t.id === template.id);
		if (i >= 0) this.items[i] = template;
		else this.items = [...this.items, template];
	}

	remove(id: number): void {
		this.items = this.items.filter((t) => t.id !== id);
	}

	clear(): void {
		this.items = [];
		this.loaded = false;
	}
}

export const templatesStore = new TemplatesStore();
