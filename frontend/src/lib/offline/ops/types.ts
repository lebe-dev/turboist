import type { OfflineOpType } from '../db';
import type { ReadCacheReader, ReadCacheWriter } from '../readCache';

// Offline op contract (FEATURE-OFFLINE-ARCH.md §4.5). This is the leaf module of
// the ops graph: the concrete ops and the registry import from here, but this
// module imports nothing from them, so there is no cycle regardless of which op
// (or the registry) a caller/test loads first.

/** HTTP request the replay engine reissues for a queued op (§4.6). */
export interface OpRequest {
	path: string;
	method: string;
	body?: unknown;
}

/** Injectable clock (ISO-8601 UTC) for deterministic synthesized timestamps. */
export type NowFn = () => string;

/**
 * One offline-replayable mutation (§4.5):
 *  - `match` recognises an outgoing request and extracts the payload to persist,
 *    or returns null when the request is not this op;
 *  - `buildRequest` reproduces the HTTP request for replay;
 *  - `synthesizeResponse` fabricates the success value returned to the caller
 *    while offline;
 *  - `applyToCache` patches cached GET responses so the change survives a restart.
 *
 * `synthesizeResponse`/`applyToCache` accept an optional `now` so tests can pin
 * the synthesized timestamps; production callers omit it and get the wall clock.
 */
export interface OfflineOp {
	readonly type: OfflineOpType;
	match(path: string, method: string, body: unknown): Record<string, unknown> | null;
	buildRequest(payload: Record<string, unknown>): OpRequest;
	synthesizeResponse(
		payload: Record<string, unknown>,
		cache: ReadCacheReader,
		now?: NowFn
	): Promise<unknown>;
	applyToCache(
		payload: Record<string, unknown>,
		cache: ReadCacheWriter,
		now?: NowFn
	): Promise<void>;
}

/**
 * Marker key an op's `match` sets (instead of a real payload) when the request
 * targets a tmp task (id < 0). Such a task exists only in this client's outbox,
 * so an operation on it cannot be replayed; `matchOp` maps the marker to a
 * `blocked` result and `tryEnqueue` answers null → `offline_unsupported`
 * (§4.5 "Блокировка операций над tmpId"). A plain string (not a Symbol) so the
 * marker compares equal across module boundaries and survives structural checks.
 */
export const BLOCKED_TMP = '__offlineBlockedTmp';
