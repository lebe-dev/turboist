import type { TaskInput } from '../../api/types';
import { insertTaskIntoInboxCache } from './patch';
import { defaultNow, taskFromInput } from './synthesize';
import { type OfflineOp, type OpRequest } from './types';

const INBOX_PATH = '/api/v1/inbox/tasks';

/**
 * `task.createInbox` — POST /api/v1/inbox/tasks. Payload `{ input, tmpId }`.
 *
 * The enqueue orchestration (C2 `tryEnqueue`) mints a negative `tmpId` via
 * `OfflineDB.nextTmpId()` and writes it into the payload *before* synthesis, so
 * the id is stable across a restart and shared with `applyToCache` (§4.5).
 * `match` only captures the `input`; the `tmpId` is added by the caller.
 */
export const taskCreateInbox: OfflineOp = {
	type: 'task.createInbox',
	match(path, method, body) {
		if (method.toUpperCase() !== 'POST') return null;
		if (path.split('?', 1)[0] !== INBOX_PATH) return null;
		const input = body && typeof body === 'object' ? (body as TaskInput) : {};
		return { input };
	},
	buildRequest(payload): OpRequest {
		return { path: INBOX_PATH, method: 'POST', body: payload.input };
	},
	async synthesizeResponse(payload, _cache, now = defaultNow) {
		const input = payload.input as TaskInput;
		const tmpId = payload.tmpId as number;
		return taskFromInput(input, tmpId, now());
	},
	async applyToCache(payload, cache, now = defaultNow) {
		const input = payload.input as TaskInput;
		const tmpId = payload.tmpId as number;
		// Same synthesized Task as synthesizeResponse, so the cached copy and the
		// value returned to the caller agree until the post-replay refetch.
		const task = taskFromInput(input, tmpId, now());
		await insertTaskIntoInboxCache(cache, task);
	}
};
