import type { TaskTemplate } from '../api/types';

class TemplatesStore {
	items = $state<TaskTemplate[]>([]);
	loaded = $state<boolean>(false);

	// Hydrated from the /api/v1/config bootstrap aggregate rather than its own
	// GET. Note the input is a bare array: ConfigResponse.taskTemplates is not
	// the Page envelope GET /api/v1/task-templates returns.
	setItems(items: TaskTemplate[]): void {
		this.items = items;
		this.loaded = true;
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
