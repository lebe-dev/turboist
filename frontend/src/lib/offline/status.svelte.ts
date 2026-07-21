import { eventsClient } from '../realtime/events.svelte';

/**
 * Reactive online/offline status for the offline module
 * (FEATURE-OFFLINE-ARCH.md §4.7). Mirrors the runes-factory idiom of
 * `events.svelte.ts`: module-private `$state` locals exposed through getters,
 * returned as a singleton.
 *
 * `online` is a heuristic. The outcome of the most recent request that reached
 * (or failed to reach) the server is the source of truth: a captive portal or
 * "Wi-Fi without internet" reports `navigator.onLine === true` yet every
 * request fails. `navigator.onLine` is only an early signal (its `offline`
 * event lets us react immediately, and it is a reliable *negative* — no network
 * interface at all), and a live SSE stream is real-time proof of connectivity.
 *
 * Only the online/servedStale/lastSyncAt/syncedAt parts land in Epic B; the
 * pending/failed counters (§4.7) arrive with the outbox in Epic C.
 */
function createStatusStore(isSseConnected: () => boolean = () => false) {
	// Authoritative signal: outcome of the most recent request. null until the
	// first request completes.
	let lastOutcomeOk = $state<boolean | null>(null);
	// Early OS signal only. Guarded for SSR where `navigator` is undefined.
	let navOnline = $state(typeof navigator === 'undefined' ? true : navigator.onLine);
	// The current view is rendered from the read-through cache (§4.3).
	let servedStale = $state(false);
	// Last successful server contact (ISO-8601 UTC); shown in the offline banner.
	let lastSyncAt = $state<string | null>(null);
	// One-shot "replay finished" signal, the analogue of
	// `eventsClient.reconnectedAt`. Driven by the replay engine in Epic C/E; it
	// lives here so subscribers can wire against it alongside `reconnectedAt`.
	let syncedAt = $state<number | null>(null);
	// Number of mutations waiting in the outbox (§4.7). The outbox is the source
	// of truth: the bridge pushes the queue length after each offline enqueue
	// (`tryEnqueue`), and the replay engine (Epic C3) after each drain.
	let pendingOps = $state(0);
	// Number of quarantined mutations in `failedOps` (§4.7). Mirrors `pendingOps`:
	// the bridge pushes the failed-store length on assembly and after each replay
	// drain (via the `onFailed` hook), and on manual retry/remove from settings.
	let failedOps = $state(0);

	if (typeof window !== 'undefined') {
		window.addEventListener('online', () => {
			navOnline = true;
		});
		window.addEventListener('offline', () => {
			navOnline = false;
		});
	}

	return {
		get online(): boolean {
			// The OS reports no network interface → definitely offline.
			if (!navOnline) return false;
			// A live SSE stream is real-time proof we are online.
			if (isSseConnected()) return true;
			// Otherwise trust the most recent actual request outcome.
			if (lastOutcomeOk !== null) return lastOutcomeOk;
			// Nothing observed yet → optimistically online.
			return true;
		},
		get servedStale(): boolean {
			return servedStale;
		},
		get lastSyncAt(): string | null {
			return lastSyncAt;
		},
		get syncedAt(): number | null {
			return syncedAt;
		},
		get pendingOps(): number {
			return pendingOps;
		},
		get failedOps(): number {
			return failedOps;
		},
		/**
		 * Record a request outcome — the authoritative online/offline signal. A
		 * success also stamps `lastSyncAt` (last time we reached the server).
		 */
		noteOutcome(ok: boolean): void {
			lastOutcomeOk = ok;
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			if (ok) lastSyncAt = new Date().toISOString();
		},
		/** A cache hit was served in place of a live response (§4.3): page is stale. */
		markStale(): void {
			servedStale = true;
		},
		/** Fresh data landed (successful GET write-through): page is live again. */
		clearStale(): void {
			servedStale = false;
		},
		/** One-shot replay-finished signal (driven by the replay engine, Epic C/E). */
		noteSynced(): void {
			syncedAt = Date.now();
		},
		/** Set the pending-op count from the outbox (its source of truth, §4.7). */
		setPendingOps(count: number): void {
			pendingOps = count;
		},
		/** Set the failed-op count from `failedOps` (its source of truth, §4.7). */
		setFailedOps(count: number): void {
			failedOps = count;
		}
	};
}

export { createStatusStore };

export const statusStore = createStatusStore(() => eventsClient.connected);
