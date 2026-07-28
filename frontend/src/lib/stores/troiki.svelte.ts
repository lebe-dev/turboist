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

// replaceTaskInPlace swaps a task inside the project that already holds it,
// preserving its index. Returns false when no slot holds that project, or when the
// project is there but does not hold the task yet (it moved projects) — the caller
// then falls back to remove-and-insert.
function replaceTaskInPlace(v: TroikiViewResponse, task: Task): boolean {
	for (const cat of CATEGORIES) {
		const slot = slotOf(v, cat);
		const projectIdx = slot.projects.findIndex((p) => p.id === task.projectId);
		if (projectIdx === -1) continue;
		const target = slot.projects[projectIdx];
		const taskIdx = target.tasks.findIndex((t) => t.id === task.id);
		if (taskIdx === -1) return false;
		const tasks = target.tasks.slice();
		tasks[taskIdx] = task;
		slot.projects = slot.projects.map((p, i) => (i === projectIdx ? { ...target, tasks } : p));
		return true;
	}
	return false;
}

class TroikiStore {
	value = $state<TroikiViewResponse>(EMPTY);

	// The Troiki page has no `useListMutator`, so this store owns the same two
	// guards `usePageLoad` applies to list views. Both exist because the page
	// revalidates on every SSE `tasks` invalidation — including the burst fired
	// when a long-idle tab reconnects, which lands right as the user reaches for a
	// checkbox.
	//
	//   writeSeq — only the most recently ISSUED write applies. Two `load()`s can
	//     be in flight (a background revalidation plus the reload `onTaskToggle`
	//     does after completing a task); without this the older one can resolve
	//     last and put the completed task back.
	//   epoch — a local optimistic edit vetoes any read that was already in flight,
	//     since that read is a snapshot from before the edit.
	private writeSeq = 0;
	private epoch = 0;

	async load(): Promise<TroikiViewResponse> {
		const my = ++this.writeSeq;
		const at = this.epoch;
		const v = await troikiApi.view(getApiClient());
		if (my === this.writeSeq && at === this.epoch) this.value = v;
		return v;
	}

	setValue(v: TroikiViewResponse): void {
		++this.writeSeq;
		this.value = v;
	}

	async start(): Promise<TroikiViewResponse> {
		const my = ++this.writeSeq;
		const v = await troikiApi.start(getApiClient());
		if (my === this.writeSeq) this.value = v;
		return v;
	}

	async reset(): Promise<TroikiViewResponse> {
		const my = ++this.writeSeq;
		const v = await troikiApi.reset(getApiClient());
		if (my === this.writeSeq) this.value = v;
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
		++this.epoch;
		const next = clone(this.value);
		// Same project as before: swap the row where it already sits, so the list keeps
		// the ordering the server gave it. The move path below removes and re-appends,
		// which on a plain field edit (day part, due date, …) would shove the row to the
		// bottom of its project card — reading as "the task disappeared" — and leave it
		// there, because this tab's own mutation raises no SSE invalidation to refetch.
		if (task.projectId !== null && replaceTaskInPlace(next, task)) {
			this.value = next;
			return;
		}
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
		++this.epoch;
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
		++this.epoch;
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
		++this.epoch;
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
