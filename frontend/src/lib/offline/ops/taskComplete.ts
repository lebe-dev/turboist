import type { Task, TaskStatus } from '../../api/types';
import { patchCachedTask } from './patch';
import { defaultNow, minimalTask, readCompletedAt } from './synthesize';
import { BLOCKED_TMP, type OfflineOp, type OpRequest } from './types';

const COMPLETE_PATH = /^\/api\/v1\/tasks\/(-?\d+)\/complete$/;

/**
 * `task.complete` — POST /api/v1/tasks/:id/complete (id > 0). Payload
 * `{ taskId, completedAt? }`.
 *
 * Note: completing a *recurring* task on the server advances its RRULE
 * (`internal/service/complete.go`); the synthesized copy does NOT — it is merely
 * marked complete. The post-replay refetch produces the next occurrence (§4.5).
 */
export const taskComplete: OfflineOp = {
	type: 'task.complete',
	match(path, method, body) {
		if (method.toUpperCase() !== 'POST') return null;
		const m = COMPLETE_PATH.exec(path.split('?', 1)[0]);
		if (!m) return null;
		const taskId = Number(m[1]);
		// A tmp task (id < 0) exists only in this outbox — cannot be completed offline.
		if (taskId < 0) return { taskId, [BLOCKED_TMP]: true };
		const completedAt = readCompletedAt(body);
		return completedAt !== undefined ? { taskId, completedAt } : { taskId };
	},
	// Offline mirror of the backend's blocker guard (internal/service/complete.go):
	// a task with open blockers cannot be completed, so it must not be queued either.
	// Enqueuing it anyway would show the task as done, then have the server reject the
	// replay and dump the op into the quarantine — a worse outcome than refusing now.
	//
	// The cache can be stale (a blocker completed on another device), in which case the
	// replay rejection + quarantine remains the backstop.
	async guard(payload, cache) {
		const cached = (await cache.findTask(payload.taskId as number)) as Task | null;
		return !cached || (cached.blockedByCount ?? 0) === 0;
	},
	buildRequest(payload): OpRequest {
		const taskId = payload.taskId as number;
		const completedAt = payload.completedAt as string | undefined;
		return {
			path: `/api/v1/tasks/${taskId}/complete`,
			method: 'POST',
			body: completedAt !== undefined ? { completedAt } : undefined
		};
	},
	async synthesizeResponse(payload, cache, now = defaultNow) {
		const taskId = payload.taskId as number;
		const ts = now();
		const completedAt = (payload.completedAt as string | undefined) ?? ts;
		const cached = (await cache.findTask(taskId)) as Task | null;
		if (cached) {
			return { ...cached, status: 'completed' as TaskStatus, completedAt };
		}
		return minimalTask(taskId, 'completed', completedAt, ts);
	},
	async applyToCache(payload, cache, now = defaultNow) {
		const taskId = payload.taskId as number;
		// Mirror synthesizeResponse: fall back to `now()` when no explicit stamp.
		const completedAt = (payload.completedAt as string | undefined) ?? now();
		await patchCachedTask(cache, taskId, (t) => {
			t.status = 'completed';
			t.completedAt = completedAt;
		});
	}
};
