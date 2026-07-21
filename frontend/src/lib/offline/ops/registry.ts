import type { OfflineOpType } from '../db';
import { taskComplete } from './taskComplete';
import { taskCreateInbox } from './taskCreateInbox';
import { taskUncomplete } from './taskUncomplete';
import { BLOCKED_TMP, type OfflineOp } from './types';

// Op registry (FEATURE-OFFLINE-ARCH.md §4.5): the public entry point the
// mutation-enqueue path (C2 `tryEnqueue`) and the replay engine (C3) use to route
// a request to its op. The `OfflineOp` contract itself lives in `./types` and is
// re-exported here so consumers have a single import surface.

export { BLOCKED_TMP } from './types';
export type { OfflineOp, OpRequest, NowFn } from './types';
export { taskComplete } from './taskComplete';
export { taskUncomplete } from './taskUncomplete';
export { taskCreateInbox } from './taskCreateInbox';

/** Every offline-replayable op, in registration order. */
export const offlineOps: readonly OfflineOp[] = [taskComplete, taskUncomplete, taskCreateInbox];

const byType = new Map<OfflineOpType, OfflineOp>(offlineOps.map((op) => [op.type, op]));

/**
 * Look up an op by its stored `QueuedOp.type` — the replay engine (§4.6) uses this
 * to reconstruct the HTTP request for a queued op. Returns undefined for an
 * unknown type (e.g. a queue row from a newer bundle that was rolled back).
 */
export function opByType(type: OfflineOpType): OfflineOp | undefined {
	return byType.get(type);
}

/** Result of routing a request through the op registry. */
export type MatchResult =
	| { kind: 'op'; op: OfflineOp; payload: Record<string, unknown> }
	| { kind: 'blocked' }
	| null;

/**
 * Route an outgoing mutation to its `OfflineOp` (§4.5). Returns:
 *  - `{ kind: 'op', op, payload }` — a whitelisted op plus its extracted payload;
 *  - `{ kind: 'blocked' }` — the op targets a tmp task (id < 0); the caller must
 *    answer `offline_unsupported`;
 *  - `null` — no op handles this request (not queueable offline).
 */
export function matchOp(path: string, method: string, body: unknown): MatchResult {
	for (const op of offlineOps) {
		const payload = op.match(path, method, body);
		if (payload === null) continue;
		if (payload[BLOCKED_TMP] === true) return { kind: 'blocked' };
		return { kind: 'op', op, payload };
	}
	return null;
}
