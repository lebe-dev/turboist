import { saveTaskSnapshot } from './db';
import type { Task, Meta } from '$lib/api/types';

// Debounce IDB writes for deltas (at most once per 2 seconds)
let writeTimer: ReturnType<typeof setTimeout> | null = null;
let pendingWrite: { view: string; contextId: string | undefined; tasks: Task[]; meta: Meta } | null =
	null;

function flushPending(): void {
	if (!pendingWrite) return;
	const { view, contextId, tasks, meta } = pendingWrite;
	pendingWrite = null;
	saveTaskSnapshot(view, contextId, tasks, meta).catch(console.error);
}

export function scheduleSnapshotWrite(
	view: string,
	contextId: string | undefined,
	tasks: Task[],
	meta: Meta
): void {
	pendingWrite = { view, contextId, tasks, meta };
	if (writeTimer) clearTimeout(writeTimer);
	writeTimer = setTimeout(() => {
		writeTimer = null;
		flushPending();
	}, 2000);
}

export function writeSnapshotImmediate(
	view: string,
	contextId: string | undefined,
	tasks: Task[],
	meta: Meta
): void {
	if (writeTimer) {
		clearTimeout(writeTimer);
		writeTimer = null;
	}
	pendingWrite = null;
	saveTaskSnapshot(view, contextId, tasks, meta).catch(console.error);
}

// Flush on visibility change (user leaving the tab/app)
if (typeof document !== 'undefined') {
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState === 'hidden' && writeTimer) {
			clearTimeout(writeTimer);
			writeTimer = null;
			flushPending();
		}
	});
}
