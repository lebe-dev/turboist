/**
 * One-shot catch-up refetch subscription (FEATURE-OFFLINE-ARCH.md §4.8).
 *
 * Two independent, monotonic signals ask the workspace to re-pull server truth
 * after the client rejoins it:
 *
 * - `eventsClient.reconnectedAt` — the SSE stream reconnected after a drop, and
 *   the server keeps no replay, so the view may have missed pushes.
 * - `statusStore.syncedAt` — the offline outbox finished replaying, so the data
 *   the view is showing has just changed on the server.
 *
 * Each is a timestamp (`number | null`) bumped once per event. This hook
 * subscribes to one such signal, dedupes against the last value it acted on,
 * and runs `refetch` when the signal advances to a fresh, non-null value.
 *
 * Wire the same `refetch` through this hook once per signal. Reconnect and
 * replay-complete can fire in either order (§4.6/§4.8) and a double refetch
 * when both fire is acceptable — both are cheap GETs and correctness beats
 * minimising requests.
 *
 * Must be called during component initialisation (it opens an `$effect`).
 */
export function useCatchupRefetch(signal: () => number | null, refetch: () => void): void {
	// Non-reactive dedupe latch: the effect's only reactive dependency is
	// `signal()`, so it re-runs exactly when the signal changes.
	let last: number | null = null;
	$effect(() => {
		const at = signal();
		if (at === null || at === last) return;
		last = at;
		refetch();
	});
}
