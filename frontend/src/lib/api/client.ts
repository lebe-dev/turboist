import { ApiError, isApiErrorEnvelope } from './errors';

export interface ApiLogEntry {
	timestamp: Date;
	method: string;
	url: string;
	status: number | null;
	durationMs: number;
	error: string | null;
	requestBody: string | null;
	responseBody: string | null;
}

/**
 * Bridge into the optional offline layer (`lib/offline`, FEATURE-OFFLINE-ARCH.md
 * §4.4). When absent, `ApiClient` is purely online and behaves exactly as before.
 * The concrete implementation lives in `lib/offline` and is wired at app
 * bootstrap; `client.ts` depends only on this interface so it never pulls the
 * IndexedDB/`idb` code into the core bundle.
 */
export interface OfflineBridge {
	/** True when the online/offline heuristic currently believes we are offline. */
	isOffline(): boolean;
	/** Look up a cached GET response by (path, query); null on a miss. */
	cacheGet(path: string, query: unknown): Promise<{ payload: unknown; storedAt: string } | null>;
	/** Write a successful GET response through to the cache. */
	cachePut(path: string, query: unknown, payload: unknown): Promise<void>;
	/**
	 * Enqueue a whitelisted mutation (§4.4/§4.5) and synthesize a success response,
	 * or null when the operation is not supported offline (not whitelisted, targets
	 * a tmp task, or no IndexedDB). `fetch` calls this on a status-0 mutation and
	 * turns null into an `offline_unsupported` ApiError.
	 */
	tryEnqueue(
		path: string,
		method: string,
		body: unknown,
		idempotencyKey?: string
	): Promise<{ response: unknown } | null>;
	/** Signal for the online/offline heuristic: a request succeeded (true) or hit a network error (false). */
	noteRequestOutcome(ok: boolean): void;
}

export interface ApiClientOptions {
	baseUrl?: string;
	getAccessToken: () => string | null;
	setAccessToken: (token: string | null) => void;
	onRefreshFailure: () => void;
	fetchImpl?: typeof fetch;
	onLog?: (entry: ApiLogEntry) => void;
	// clientOrigin tags mutating requests with the `X-Client-Origin` header so
	// the backend can suppress this client's own SSE echoes. Empty disables it.
	clientOrigin?: string;
	// onMutation fires after a successful (2xx) mutating request. Because the
	// originating client no longer receives the SSE echo of its own change, this
	// is the hook that refreshes derived data (sidebar counts etc.) locally.
	onMutation?: (path: string, method: string) => void;
	// Native refresh-token accessors. Undefined on web, where the rotating token
	// lives in an HttpOnly cookie sent via credentials:'include'. When provided
	// (native), performRefresh sends the stored token in the request body and
	// persists the rotated token from the response.
	getRefreshToken?: () => Promise<string | null>;
	setRefreshToken?: (token: string | null) => Promise<void>;
	// Optional offline layer. Undefined → purely-online behaviour (unchanged).
	offline?: OfflineBridge;
}

export type QueryValue = string | number | boolean | undefined | null;

interface ApiFetchInit extends Omit<RequestInit, 'body'> {
	body?: unknown;
	query?: Record<string, QueryValue> | object;
	skipAuth?: boolean;
	skipRefresh?: boolean;
	// Set by the offline replay engine so a replayed request neither reads the
	// read-through cache nor (Epic C) re-enqueues itself when the network drops.
	skipOffline?: boolean;
	// Idempotency key carried by a replayed mutation (Epic C); unused on the B3 read path.
	idempotencyKey?: string;
}

interface RefreshResponseBody {
	access: string;
	refresh?: string;
}

export class ApiClient {
	readonly baseUrl: string;
	private readonly getAccessToken: () => string | null;
	private readonly setAccessToken: (token: string | null) => void;
	private readonly onRefreshFailure: () => void;
	private readonly fetchImpl: typeof fetch;
	private readonly onLog?: (entry: ApiLogEntry) => void;
	private readonly clientOrigin?: string;
	private readonly onMutation?: (path: string, method: string) => void;
	private readonly getRefreshToken?: () => Promise<string | null>;
	private readonly setRefreshToken?: (token: string | null) => Promise<void>;
	private readonly offline?: OfflineBridge;
	private refreshInflight: Promise<string | null> | null = null;

	constructor(options: ApiClientOptions) {
		this.baseUrl = options.baseUrl ?? '';
		this.getAccessToken = options.getAccessToken;
		this.setAccessToken = options.setAccessToken;
		this.onRefreshFailure = options.onRefreshFailure;
		this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
		this.onLog = options.onLog;
		this.clientOrigin = options.clientOrigin;
		this.onMutation = options.onMutation;
		this.getRefreshToken = options.getRefreshToken;
		this.setRefreshToken = options.setRefreshToken;
		this.offline = options.offline;
	}

	private static readonly mutatingMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

	async fetch<T>(path: string, init: ApiFetchInit = {}): Promise<T> {
		const url = this.buildUrl(path, init.query);
		const method = (init.method ?? 'GET').toUpperCase();
		const isMutation = ApiClient.mutatingMethods.has(method);
		const reqBody = init.body != null
			? (typeof init.body === 'string' ? init.body : JSON.stringify(init.body))
			: null;
		const start = performance.now();

		// Idempotency-Key on every mutation while the offline layer is active
		// (§4.4 step 1). One key per fetch() call: setting it on init.headers here
		// means doRequest — which rebuilds Headers from init on each attempt —
		// reuses the SAME key on its internal 401 retry, so a refresh-and-retry
		// never double-executes on the backend. A replayed op passes its stored
		// key via init.idempotencyKey; otherwise we mint a fresh UUID. Purely-online
		// builds (no bridge) add nothing, keeping their behaviour bit-for-bit.
		// The exact key sent to the server, captured so a lost-response mutation is
		// enqueued and replayed under THE SAME key (§6 scenario 3) — otherwise the
		// server would not recognise the replay and would re-execute (duplicate task
		// / double RRULE advance).
		let idempotencyKey: string | undefined;
		if (isMutation && this.offline) {
			const headers = new Headers(init.headers ?? {});
			if (!headers.has('Idempotency-Key')) {
				headers.set('Idempotency-Key', init.idempotencyKey ?? crypto.randomUUID());
				init = { ...init, headers };
			}
			idempotencyKey = headers.get('Idempotency-Key') ?? undefined;
		}

		// Cache-first when we already believe we are offline (GET only): return the
		// cached payload immediately without waiting on the 15s network timeout, and
		// fire a background probe that refreshes the cache and flips us back online on
		// success. skipOffline (replay engine) opts a request out of the cache entirely.
		if (!isMutation && !init.skipOffline && this.offline?.isOffline()) {
			const hit = await this.offline.cacheGet(path, init.query);
			if (hit) {
				this.emitLog(method, url, null, start, reqBody, hit.payload, null);
				this.probeNetworkInBackground(path, init);
				return hit.payload as T;
			}
		}

		try {
			const response = await this.doRequest(url, init, /*isRetry*/ false);
			const result = await this.parseResponse<T>(response);
			this.emitLog(method, url, response.status, start, reqBody, result, null);
			this.offline?.noteRequestOutcome(true);
			if (!isMutation && this.offline) {
				await this.offline.cachePut(path, init.query, result);
			}
			if (this.onMutation && isMutation) {
				this.onMutation(path, method);
			}
			return result;
		} catch (err) {
			// A network error (status 0) with the offline layer active: serve a stale
			// GET from cache, or queue a whitelisted mutation and hand back a
			// synthesized success (§4.4). skipOffline (replay reissue) opts out.
			if (err instanceof ApiError && err.status === 0 && this.offline && !init.skipOffline) {
				this.offline.noteRequestOutcome(false);
				if (!isMutation) {
					const hit = await this.offline.cacheGet(path, init.query);
					if (hit) {
						this.emitLog(method, url, null, start, reqBody, hit.payload, null);
						return hit.payload as T;
					}
				} else {
					// The bridge enqueues the op and synthesizes its response. We do NOT
					// fire onMutation — the change is not on the server yet, and the
					// outbox owns the pending-count update; firing it would kick a
					// pointless offline refetch storm. A non-whitelisted op → null →
					// offline_unsupported, surfaced by describeError.
					const queued = await this.offline.tryEnqueue(path, method, init.body, idempotencyKey);
					if (queued) {
						this.emitLog(method, url, null, start, reqBody, queued.response, null);
						return queued.response as T;
					}
					throw new ApiError('offline_unsupported', 'action unavailable offline', 0);
				}
			}
			const status = err instanceof ApiError ? err.status : null;
			const errMsg = err instanceof Error ? err.message : String(err);
			this.emitLog(method, url, status, start, reqBody, null, errMsg);
			throw err;
		}
	}

	/**
	 * Fire-and-forget network probe used after a cache-first hit while offline: on
	 * success it refreshes the read-through cache and flips the heuristic back to
	 * online; on failure we stay offline (the foreground already served cache).
	 */
	private probeNetworkInBackground(path: string, init: ApiFetchInit): void {
		if (!this.offline) return;
		const url = this.buildUrl(path, init.query);
		void (async () => {
			try {
				const response = await this.doRequest(url, init, /*isRetry*/ false);
				const result = await this.parseResponse<unknown>(response);
				this.offline?.noteRequestOutcome(true);
				await this.offline?.cachePut(path, init.query, result);
			} catch {
				// Probe failed — remain offline; the caller was already served from cache.
			}
		})();
	}

	private emitLog(
		method: string,
		url: string,
		status: number | null,
		start: number,
		requestBody: string | null,
		result: unknown,
		error: string | null
	): void {
		if (!this.onLog) return;
		let responseBody: string | null = null;
		if (result !== undefined && result !== null) {
			try {
				const s = JSON.stringify(result);
				responseBody = s.length > 2000 ? s.slice(0, 2000) + '…' : s;
			} catch { /* skip */ }
		}
		this.onLog({
			timestamp: new Date(),
			method,
			url,
			status: status || null,
			durationMs: Math.round(performance.now() - start),
			error,
			requestBody: requestBody ? (requestBody.length > 2000 ? requestBody.slice(0, 2000) + '…' : requestBody) : null,
			responseBody
		});
	}

	private buildUrl(
		path: string,
		query: ApiFetchInit['query']
	): string {
		const base = this.baseUrl + path;
		const qs = canonicalizeQuery(query);
		return qs ? `${base}?${qs}` : base;
	}

	private async doRequest(
		url: string,
		init: ApiFetchInit,
		isRetry: boolean
	): Promise<Response> {
		const headers = new Headers(init.headers ?? {});
		let body: BodyInit | undefined;
		if (init.body !== undefined && init.body !== null) {
			if (
				init.body instanceof FormData ||
				init.body instanceof Blob ||
				typeof init.body === 'string'
			) {
				body = init.body as BodyInit;
			} else {
				headers.set('Content-Type', 'application/json');
				body = JSON.stringify(init.body);
			}
		}

		if (!init.skipAuth) {
			const token = this.getAccessToken();
			if (token) headers.set('Authorization', `Bearer ${token}`);
		}

		// Tag mutations so the backend can suppress this client's own SSE echo.
		if (this.clientOrigin && ApiClient.mutatingMethods.has((init.method ?? 'GET').toUpperCase())) {
			headers.set('X-Client-Origin', this.clientOrigin);
		}

		let response: Response;
		try {
			const signal = init.signal ?? AbortSignal.timeout(15_000);
			response = await this.fetchImpl(url, {
				...init,
				headers,
				body,
				signal,
				credentials: init.credentials ?? 'same-origin'
			});
		} catch (err) {
			if (err instanceof DOMException && err.name === 'TimeoutError') {
				throw new ApiError('timeout', 'request timed out', 0);
			}
			throw new ApiError(
				'network_error',
				err instanceof Error ? err.message : 'network error',
				0
			);
		}

		if (response.status !== 401 || init.skipRefresh || isRetry) {
			return response;
		}

		const errorPayload = await this.peekErrorCode(response);
		if (errorPayload !== 'auth_expired') {
			return response;
		}

		const newToken = await this.refreshAccessToken();
		if (newToken === null) {
			return response;
		}
		return this.doRequest(url, init, true);
	}

	private async peekErrorCode(response: Response): Promise<string | null> {
		try {
			const cloned = response.clone();
			const data = await cloned.json();
			if (isApiErrorEnvelope(data)) return data.error.code;
		} catch {
			return null;
		}
		return null;
	}

	private refreshAccessToken(): Promise<string | null> {
		if (this.refreshInflight) return this.refreshInflight;
		this.refreshInflight = this.performRefresh().finally(() => {
			this.refreshInflight = null;
		});
		return this.refreshInflight;
	}

	private async performRefresh(): Promise<string | null> {
		const headers = new Headers();
		let body: string | undefined;
		// Native: send the persisted rotating token in the body (the backend reads
		// body when there is no cookie). Web: no body — the HttpOnly cookie rides
		// on credentials:'include'.
		if (this.getRefreshToken) {
			const stored = await this.getRefreshToken();
			if (!stored) {
				this.setAccessToken(null);
				this.onRefreshFailure();
				return null;
			}
			headers.set('Content-Type', 'application/json');
			body = JSON.stringify({ refresh: stored });
		}
		let response: Response;
		try {
			response = await this.fetchImpl(this.baseUrl + '/auth/refresh', {
				method: 'POST',
				credentials: 'include',
				headers,
				body
			});
		} catch {
			// Network error: leave session intact; caller will see the original 401 and surface it.
			return null;
		}
		if (!response.ok) {
			// Only treat actual auth rejection as a forced logout; transient 5xx must not log the user out.
			if (response.status === 401 || response.status === 403) {
				this.setAccessToken(null);
				if (this.setRefreshToken) await this.setRefreshToken(null);
				this.onRefreshFailure();
			}
			return null;
		}
		const data = (await response.json()) as RefreshResponseBody;
		this.setAccessToken(data.access);
		if (this.setRefreshToken && data.refresh) await this.setRefreshToken(data.refresh);
		return data.access;
	}

	private async parseResponse<T>(response: Response): Promise<T> {
		if (response.status === 204) {
			return undefined as T;
		}
		const text = await response.text();
		const data = text ? safeJsonParse(text) : null;

		if (!response.ok) {
			if (isApiErrorEnvelope(data)) {
				throw new ApiError(
					data.error.code,
					data.error.message,
					response.status,
					data.error.details
				);
			}
			throw new ApiError(
				'unknown_error',
				`HTTP ${response.status}`,
				response.status
			);
		}
		return data as T;
	}
}

function safeJsonParse(text: string): unknown {
	try {
		return JSON.parse(text);
	} catch {
		return null;
	}
}

/**
 * Canonicalize a query object into a stable query string: keys are sorted and
 * null/undefined values dropped. Used both to build request URLs (`buildUrl`)
 * and, via `buildCacheKey` in `lib/offline/db.ts`, to derive the offline
 * read-through cache key — so a request and its cache entry always agree
 * regardless of key insertion order (FEATURE-OFFLINE-ARCH.md §4.2/§4.3).
 * Returns '' (no leading '?') when there is nothing to append.
 */
export function canonicalizeQuery(query: unknown): string {
	if (query === null || query === undefined || typeof query !== 'object') return '';
	const source = query as Record<string, unknown>;
	const params = new URLSearchParams();
	for (const key of Object.keys(source).sort()) {
		const value = source[key];
		if (value === undefined || value === null) continue;
		params.append(key, String(value));
	}
	return params.toString();
}

let clientInstance: ApiClient | null = null;

export function setApiClient(client: ApiClient): void {
	clientInstance = client;
}

export function getApiClient(): ApiClient {
	if (!clientInstance) {
		throw new Error('ApiClient is not initialised. Call setApiClient first.');
	}
	return clientInstance;
}
