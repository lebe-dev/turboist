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
 * eventsClient is a singleton SSE subscriber. It manages:
 *   - obtaining a short-lived ticket via the REST API,
 *   - opening an EventSource with that ticket,
 *   - reconnecting with exponential backoff after disconnects,
 *   - dispatching parsed events to handlers registered with `on(scope, ...)`.
 *
 * The native EventSource does its own reconnect on transient HTTP errors, but
 * we always tear down and re-handshake after a disconnect so we re-issue a
 * fresh ticket (server-side tickets are single-use). We also expose
 * `reconnectedAt` so consumers can do a one-shot catch-up refetch after the
 * gap.
 */
function createEventsClient() {
	let source: EventSource | null = null;
	let started = false;
	let connecting = false;
	let attempt = 0;
	let retryTimer: ReturnType<typeof setTimeout> | null = null;
	let lastDisconnectAt = $state<number | null>(null);

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

	function teardown(): void {
		if (source) {
			source.close();
			source = null;
		}
		clearTimer();
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
				attempt = 0;
				connected = true;
				if (lastDisconnectAt !== null) {
					reconnectedAt = Date.now();
					lastDisconnectAt = null;
				}
			};
			es.onerror = () => {
				if (connected) {
					lastDisconnectAt = Date.now();
				}
				connected = false;
				// EventSource transitions to CLOSED on auth failure or after the
				// browser gives up. Tear down and reschedule with our own backoff
				// so we re-issue a ticket.
				if (es.readyState === EventSource.CLOSED) {
					if (source === es) {
						source = null;
					}
					es.close();
					scheduleReconnect();
				}
			};
			for (const scope of handlers.keys()) {
				attachScopeListener(es, scope);
			}
		} catch (err) {
			console.warn('events: handshake failed', err);
			scheduleReconnect();
		} finally {
			connecting = false;
		}
	}

	function attachScopeListener(es: EventSource, scope: EventScope): void {
		es.addEventListener(scope, () => {
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
		 * (visibilitychange → visible, `online`).
		 *
		 * A hidden tab's timers are throttled to minutes, so the reconnect scheduled
		 * when the stream dropped fires only once the tab is foregrounded again —
		 * seconds after the user is already reaching for a checkbox. Its catch-up
		 * refresh then lands mid-interaction, which is exactly the "the page
		 * refreshed and my click did nothing" symptom. Reconnecting eagerly moves
		 * that refresh into the wake itself. Resetting `attempt` also discards
		 * backoff accumulated while the device had no network at all.
		 *
		 * A healthy, open stream is left alone.
		 */
		resume(): void {
			if (!started || connected) return;
			if (source && source.readyState === EventSource.OPEN) return;
			attempt = 0;
			// Drops a source stuck in CONNECTING (the native retry after a suspend)
			// so connect() re-handshakes for a fresh single-use ticket.
			teardown();
			void connect();
		},
		stop(): void {
			started = false;
			teardown();
			connected = false;
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
