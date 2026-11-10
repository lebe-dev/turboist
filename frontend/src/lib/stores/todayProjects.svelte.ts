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

	return {
		get ids(): number[] {
			return ids;
		},
		set(next: number[]): void {
			ids = next;
		},
		clear(): void {
			ids = [];
		}
	};
}

export const todayProjectsStore = createTodayProjectsStore();
