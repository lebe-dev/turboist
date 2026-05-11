import { troiki as troikiApi } from '../api/endpoints/troiki';
import { getApiClient } from '../api/client';
import type {
	Project,
	Task,
	TroikiCategory,
	TroikiProject,
	TroikiSlot,
	TroikiViewResponse
} from '../api/types';

const EMPTY: TroikiViewResponse = {
	important: { capacity: 3, projects: [] },
	medium: { capacity: 0, projects: [] },
	rest: { capacity: 0, projects: [] },
	started: false
};

const CATEGORIES: TroikiCategory[] = ['important', 'medium', 'rest'];

function clone(v: TroikiViewResponse): TroikiViewResponse {
	return {
		important: { capacity: v.important.capacity, projects: v.important.projects.slice() },
		medium: { capacity: v.medium.capacity, projects: v.medium.projects.slice() },
		rest: { capacity: v.rest.capacity, projects: v.rest.projects.slice() },
		started: v.started
	};
}

function slotOf(v: TroikiViewResponse, cat: TroikiCategory): TroikiSlot {
	return v[cat];
}

class TroikiStore {
	value = $state<TroikiViewResponse>(EMPTY);

	async load(): Promise<TroikiViewResponse> {
		const v = await troikiApi.view(getApiClient());
		this.value = v;
		return v;
	}

	async start(): Promise<TroikiViewResponse> {
		const v = await troikiApi.start(getApiClient());
		this.value = v;
		return v;
	}

	async reset(): Promise<TroikiViewResponse> {
		const v = await troikiApi.reset(getApiClient());
		this.value = v;
		return v;
	}

	clear(): void {
		this.value = EMPTY;
	}

	// applyTaskUpdate mutates the task within whatever Troiki project currently owns it.
	// If the task moved to a different project, it is removed from the old project and
	// inserted into the new one (when that project sits in any slot). Completed tasks
	// stay visible under their project — the backend view includes them so users can
	// see what they finished in the current cycle.
	applyTaskUpdate(task: Task): void {
		const next = clone(this.value);
		for (const cat of CATEGORIES) {
			const slot = slotOf(next, cat);
			slot.projects = slot.projects.map((p) => ({
				...p,
				tasks: p.tasks.filter((t) => t.id !== task.id)
			}));
		}
		if (task.projectId === null) {
			this.value = next;
			return;
		}
		for (const cat of CATEGORIES) {
			const slot = slotOf(next, cat);
			const idx = slot.projects.findIndex((p) => p.id === task.projectId);
			if (idx !== -1) {
				const target = slot.projects[idx];
				slot.projects[idx] = { ...target, tasks: [...target.tasks, task] };
				break;
			}
		}
		this.value = next;
	}

	// applyProjectUpdate moves a project between slots when its category changes,
	// drops it when category is cleared, and refreshes its metadata in place. Tasks
	// already attached to the project are preserved across moves.
	applyProjectUpdate(project: Project): void {
		const next = clone(this.value);
		let existingTasks: Task[] = [];
		for (const cat of CATEGORIES) {
			const slot = slotOf(next, cat);
			const idx = slot.projects.findIndex((p) => p.id === project.id);
			if (idx !== -1) {
				existingTasks = slot.projects[idx].tasks;
				slot.projects = slot.projects.filter((p) => p.id !== project.id);
			}
		}
		const targetCat = project.troikiCategory;
		if (targetCat && CATEGORIES.includes(targetCat)) {
			const slot = slotOf(next, targetCat);
			const merged: TroikiProject = { ...project, tasks: existingTasks };
			slot.projects = [...slot.projects, merged];
		}
		this.value = next;
	}

	// insertTaskAfter adds a task into its owning Troiki project right after
	// the given reference task. Used by duplicate flow so the new task shows
	// up without a full refetch. If the reference is not found within the
	// project, the task is appended to the end.
	insertTaskAfter(referenceId: number, task: Task): void {
		if (task.projectId === null) return;
		const next = clone(this.value);
		for (const cat of CATEGORIES) {
			const slot = slotOf(next, cat);
			const idx = slot.projects.findIndex((p) => p.id === task.projectId);
			if (idx === -1) continue;
			const target = slot.projects[idx];
			if (target.tasks.some((t) => t.id === task.id)) {
				this.value = next;
				return;
			}
			const refIdx = target.tasks.findIndex((t) => t.id === referenceId);
			const insertAt = refIdx === -1 ? target.tasks.length : refIdx + 1;
			const tasks = [
				...target.tasks.slice(0, insertAt),
				task,
				...target.tasks.slice(insertAt)
			];
			slot.projects[idx] = { ...target, tasks };
			this.value = next;
			return;
		}
		this.value = next;
	}

	removeTask(id: number): void {
		const next = clone(this.value);
		for (const cat of CATEGORIES) {
			const slot = slotOf(next, cat);
			slot.projects = slot.projects.map((p) => ({
				...p,
				tasks: p.tasks.filter((t) => t.id !== id)
			}));
		}
		this.value = next;
	}
}

export const troikiStore = new TroikiStore();
