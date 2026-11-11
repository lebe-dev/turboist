// Projects of the tasks on the Today screen, in the order they first appear.
//
// The Today page publishes them while it is mounted so the quick-add dialog,
// which lives in the app layout and cannot see the page, can offer them as
// one-tap project choices. Empty everywhere else, which is what keeps the row
// out of the dialog on other screens.

/** Distinct project ids of `tasks`, first appearance first. Inbox tasks are skipped. */
export function distinctProjectIds(tasks: Array<{ projectId: number | null }>): number[] {
	const ids: number[] = [];
	for (const task of tasks) {
		if (task.projectId === null || ids.includes(task.projectId)) continue;
		ids.push(task.projectId);
	}
	return ids;
}

function createTodayProjectsStore() {
	let ids = $state<number[]>([]);
	let known = $state(false);

	return {
		get ids(): number[] {
			return ids;
		},
		/** Whether the Today page is mounted and `ids` is the real set — an empty
		 * list then means Today has no project, not that nobody published one. */
		get known(): boolean {
			return known;
		},
		set(next: number[]): void {
			ids = next;
			known = true;
		},
		clear(): void {
			ids = [];
			known = false;
		}
	};
}

export const todayProjectsStore = createTodayProjectsStore();
