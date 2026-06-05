import { projects as projectsApi } from '../api/endpoints/projects';
import { getApiClient } from '../api/client';
import type { Project } from '../api/types';
import { dropTombstones, isLive } from '../utils/tombstone';

class ProjectsStore {
	items = $state<Project[]>([]);
	loaded = $state<boolean>(false);

	pinned = $derived(this.items.filter((p) => p.isPinned));

	byContext(contextId: number): Project[] {
		return this.items.filter((p) => p.contextId === contextId);
	}

	async load(): Promise<Project[]> {
		const page = await projectsApi.list(getApiClient(), { limit: 500 });
		const live = dropTombstones(page.items);
		this.items = live;
		this.loaded = true;
		return live;
	}

	setItems(items: Project[]): void {
		this.items = dropTombstones(items);
		this.loaded = true;
	}

	upsert(project: Project): void {
		// A federation-pushed tombstone arrives as a soft-deleted entity; treat
		// it as a removal rather than inserting a dead row.
		if (!isLive(project)) {
			this.remove(project.id);
			return;
		}
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
