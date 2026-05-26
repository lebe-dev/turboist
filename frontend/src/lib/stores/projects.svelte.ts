import { projects as projectsApi } from '../api/endpoints/projects';
import { getApiClient } from '../api/client';
import type { Project } from '../api/types';
import { getDB } from '../offline/db';

class ProjectsStore {
	items = $state<Project[]>([]);
	loaded = $state<boolean>(false);

	pinned = $derived(this.items.filter((p) => p.isPinned));

	byContext(contextId: number): Project[] {
		return this.items.filter((p) => p.contextId === contextId);
	}

	async load(): Promise<Project[]> {
		const page = await projectsApi.list(getApiClient(), { limit: 500 });
		this.items = page.items;
		this.loaded = true;
		return page.items;
	}

	async loadCached(): Promise<boolean> {
		const rows = await getDB().table('projects').toArray();
		const items: Project[] = [];
		for (const row of rows) {
			if (row.deletedAt !== null) continue;
			items.push(row.data as Project);
		}
		if (items.length === 0) return false;
		this.items = items;
		this.loaded = true;
		return true;
	}

	upsert(project: Project): void {
		const i = this.items.findIndex((p) => p.id === project.id);
		if (i >= 0) this.items[i] = project;
		else this.items = [...this.items, project];
	}

	remove(id: number): void {
		this.items = this.items.filter((p) => p.id !== id);
	}

	clear(): void {
		this.items = [];
		this.loaded = false;
	}
}

export const projectsStore = new ProjectsStore();
