import { events as eventsApi } from '../api/endpoints/events';
import { getApiClient } from '../api/client';
import { clientOrigin } from './origin';

export type EventScope =
	| 'tasks'
	| 'calendar'
	| 'inbox'
	| 'projects'
	| 'labels'
	| 'contexts'
	| 'sections'
	| 'plan';

export type EventHandler = (scope: EventScope) => void;

const RECONNECT_BACKOFF_MS = [1_000, 2_000, 4_000, 8_000, 15_000, 30_000];

/**
 * A stream that stayed up at least this long counts as healthy, and its next
 * drop restarts the backoff from the shortest delay.
 *
 * Resetting the backoff on `open` alone is not enough: a proxy that accepts the
 * stream and kills it a second later would then reconnect once per second
 * forever, and since every reconnect reports a gap, each one also drags a
 * catch-up `GET /api/v1/config` behind it. With the threshold a flapping stream
 * backs off to the 30s step like any other failure.
 */
const HEALTHY_AFTER_MS = 10_000;

/**
 * Re-handshake when nothing at all arrives from the server for this long.
 *
 * The server sends a `ping` event every 25s (`sseHeartbeatInterval` in
 * `internal/httpapi/handlers/events.go`), so silence this long means the socket
 * is dead. The browser does NOT reliably say so: a phone that suspends its
 * radio, or a NAT/proxy that drops the connection, leaves EventSource in OPEN
 * with no error event ever firing — and without this watchdog the tab keeps
 * reporting `connected` and showing data that stopped updating hours ago.
 */
const LIVENESS_TIMEOUT_MS = 70_000;

/** Server heartbeat. Not a data scope — it only proves the stream is alive. */
const PING_EVENT = 'ping';

/**
 * eventsClient is a singleton SSE subscriber. It manages:
 *   - obtaining a short-lived ticket via the REST API,
 *   - opening an EventSource with that ticket,
 *   - reconnecting with exponential backoff after disconnects,
 *   - dispatching parsed events to handlers registered with `on(scope, ...)`.
 *
 * We never let the native EventSource do its own reconnect: the ticket sits in
 * the stream URL and is single-use (`TicketStore.Consume`), so the browser's
 * retry — which reuses that exact URL — can only ever get a 401. Every drop is
 * torn down here and re-handshaked on our own backoff.
 *
 * `reconnectedAt` exposes "there was a window in which we were not subscribed"
 * so consumers can do a one-shot catch-up refetch. It covers a failed handshake
 * as well as a lost stream — a client that boots with no network never had a
 * stream to lose, yet is showing exactly as stale a screen.
 */
function createEventsClient() {
	let source: EventSource | null = null;
	let started = false;
	let connecting = false;
	let attempt = 0;
	let retryTimer: ReturnType<typeof setTimeout> | null = null;
	let livenessTimer: ReturnType<typeof setTimeout> | null = null;
	// When the current stream opened; null while we have no live stream.
	let openedAt: number | null = null;
	// Start of the current gap in subscription coverage; null while covered.
	let lastDisconnectAt: number | null = null;

	let connected = $state(false);
	let reconnectedAt = $state<number | null>(null);

	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const handlers = new Map<EventScope, Set<EventHandler>>();

	function clearTimer(): void {
		if (retryTimer !== null) {
			clearTimeout(retryTimer);
			retryTimer = null;
		}
	}

	function clearLiveness(): void {
		if (livenessTimer !== null) {
			clearTimeout(livenessTimer);
			livenessTimer = null;
		}
	}

	function teardown(): void {
		clearLiveness();
		if (source) {
			source.close();
			source = null;
		}
		clearTimer();
	}

	/**
	 * Mark the start of a gap in subscription coverage, so the next successful
	 * open reports `reconnectedAt` and consumers run their catch-up refetch.
	 * Keeps the FIRST timestamp: a run of failed handshakes is one gap, not one
	 * per attempt.
	 */
	function noteGap(): void {
		if (lastDisconnectAt === null) lastDisconnectAt = Date.now();
	}

	/** Bookkeeping shared by every path that loses the stream. */
	function noteDrop(): void {
		if (openedAt !== null && Date.now() - openedAt >= HEALTHY_AFTER_MS) attempt = 0;
		openedAt = null;
		noteGap();
		connected = false;
	}

	function scheduleReconnect(): void {
		if (!started) return;
		const delay = RECONNECT_BACKOFF_MS[Math.min(attempt, RECONNECT_BACKOFF_MS.length - 1)];
		attempt += 1;
		clearTimer();
		retryTimer = setTimeout(() => {
			void connect();
		}, delay);
	}

	/** Drop whatever we have and re-handshake now, without waiting for a backoff. */
	function reconnectNow(): void {
		if (!started) return;
		noteDrop();
		teardown();
		void connect();
	}

	/** (Re)start the liveness watchdog. Called on every byte the server sends. */
	function armLiveness(): void {
		clearLiveness();
		livenessTimer = setTimeout(() => {
			livenessTimer = null;
			// No heartbeat for well over two intervals: the socket is dead even
			// though the browser still calls it open.
			console.warn('events: heartbeat timed out, re-handshaking');
			reconnectNow();
		}, LIVENESS_TIMEOUT_MS);
	}

	async function connect(): Promise<void> {
		if (!started || connecting || source) return;
		connecting = true;
		try {
			const { ticket } = await eventsApi.issueTicket(getApiClient(), clientOrigin);
			if (!started) return;
			// Absolute against the API base so the native EventSource targets the
			// remote server (baseUrl is '' on web → identical relative URL). This
			// is the one cross-origin GET on the WebView stack, hence the CORS
			// allow-list on /api/v1/events in the backend.
			const url = `${getApiClient().baseUrl}/api/v1/events?ticket=${encodeURIComponent(ticket)}`;
			const es = new EventSource(url);
			source = es;
			es.onopen = () => {
				if (source !== es) return;
				openedAt = Date.now();
				connected = true;
				armLiveness();
				if (lastDisconnectAt !== null) {
					reconnectedAt = Date.now();
					lastDisconnectAt = null;
				}
			};
			es.onerror = () => {
				// Whatever the readyState: the ticket in this URL has been consumed, so
				// the retry the browser may be about to make is guaranteed to 401. Drop
				// the stream and re-handshake on our own backoff instead of waiting out
				// a doomed attempt (which cost ~3s of dead realtime per transient drop).
				if (source !== es) return;
				source = null;
				noteDrop();
				clearLiveness();
				es.close();
				scheduleReconnect();
			};
			es.addEventListener(PING_EVENT, () => {
				if (source === es) armLiveness();
			});
			for (const scope of handlers.keys()) {
				attachScopeListener(es, scope);
			}
		} catch (err) {
			console.warn('events: handshake failed', err);
			// A failed handshake is a coverage gap too: whenever we do get through,
			// consumers must catch up on everything that changed while we were out.
			noteGap();
			scheduleReconnect();
		} finally {
			connecting = false;
		}
	}

	function attachScopeListener(es: EventSource, scope: EventScope): void {
		es.addEventListener(scope, () => {
			// A late event from a stream we already abandoned must neither refresh
			// views nor convince the watchdog that the current stream is alive.
			if (source !== es) return;
			armLiveness();
			const set = handlers.get(scope);
			if (!set) return;
			for (const h of set) {
				try {
					h(scope);
				} catch (err) {
					console.error(`events: handler for ${scope} threw`, err);
				}
			}
		});
	}

	return {
		get connected(): boolean {
			return connected;
		},
		get reconnectedAt(): number | null {
			return reconnectedAt;
		},
		start(): void {
			if (started) return;
			started = true;
			void connect();
		},
		/**
		 * Reconnect now instead of waiting out the backoff. Call this on wake
		 * (visibilitychange → visible, `focus`, `pageshow`, `online`).
		 *
		 * A hidden tab's timers are throttled to minutes, so the reconnect scheduled
		 * when the stream dropped fires only once the tab is foregrounded again —
		 * seconds after the user is already reaching for a checkbox. Its catch-up
		 * refresh then lands mid-interaction, which is exactly the "the page
		 * refreshed and my click did nothing" symptom. Reconnecting eagerly moves
		 * that refresh into the wake itself. Resetting `attempt` also discards
		 * backoff accumulated while the device had no network at all.
		 *
		 * A healthy, open stream is left alone — unless `force` is set, which the
		 * caller uses when the tab was in the background long enough that "open"
		 * cannot be trusted (a suspended radio kills the socket without ever firing
		 * an error). The forced re-handshake reports a gap, so the usual catch-up
		 * refetch runs and the woken tab is not left showing stale data.
		 */
		resume(force = false): void {
			if (!started) return;
			if (!force) {
				if (connected) return;
				if (source && source.readyState === EventSource.OPEN) return;
			}
			attempt = 0;
			// Drops a source stuck in CONNECTING (or an untrustworthy open one) so
			// connect() re-handshakes for a fresh single-use ticket.
			reconnectNow();
		},
		stop(): void {
			started = false;
			teardown();
			connected = false;
			openedAt = null;
			lastDisconnectAt = null;
			attempt = 0;
		},
		on(scope: EventScope, handler: EventHandler): () => void {
			let set = handlers.get(scope);
			if (!set) {
				// eslint-disable-next-line svelte/prefer-svelte-reactivity
				set = new Set();
				handlers.set(scope, set);
				if (source) attachScopeListener(source, scope);
			}
			set.add(handler);
			return () => {
				set.delete(handler);
			};
		}
	};
}

export const eventsClient = createEventsClient();
