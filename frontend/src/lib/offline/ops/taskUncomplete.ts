import type { Task, TaskStatus } from '../../api/types';
import { patchCachedTask } from './patch';
import { defaultNow, minimalTask } from './synthesize';
import { BLOCKED_TMP, type OfflineOp, type OpRequest } from './types';

const UNCOMPLETE_PATH = /^\/api\/v1\/tasks\/(-?\d+)\/uncomplete$/;

/**
 * `task.uncomplete` — POST /api/v1/tasks/:id/uncomplete (id > 0). Payload
 * `{ taskId }`. Synthesizes the task back to open with `completedAt: null`
 * (`TaskStatus` has no `active` state; an incomplete task is `open`).
 */
export const taskUncomplete: OfflineOp = {
	type: 'task.uncomplete',
	match(path, method, _body) {
		if (method.toUpperCase() !== 'POST') return null;
		const m = UNCOMPLETE_PATH.exec(path.split('?', 1)[0]);
		if (!m) return null;
		const taskId = Number(m[1]);
		if (taskId < 0) return { taskId, [BLOCKED_TMP]: true };
		return { taskId };
	},
	buildRequest(payload): OpRequest {
		const taskId = payload.taskId as number;
		return { path: `/api/v1/tasks/${taskId}/uncomplete`, method: 'POST' };
	},
	async synthesizeResponse(payload, cache, now = defaultNow) {
		const taskId = payload.taskId as number;
		const cached = (await cache.findTask(taskId)) as Task | null;
		if (cached) {
			return { ...cached, status: 'open' as TaskStatus, completedAt: null };
		}
		return minimalTask(taskId, 'open', null, now());
	},
	async applyToCache(payload, cache) {
		const taskId = payload.taskId as number;
		await patchCachedTask(cache, taskId, (t) => {
			t.status = 'open';
			t.completedAt = null;
		});
	}
};
