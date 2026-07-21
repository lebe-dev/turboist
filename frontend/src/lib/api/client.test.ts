import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, canonicalizeQuery, type OfflineBridge } from './client';
import { ApiError } from './errors';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function emptyResponse(status = 204): Response {
	return new Response(null, { status });
}

interface ClientHarness {
	client: ApiClient;
	fetchMock: ReturnType<typeof vi.fn>;
	tokens: { access: string | null; refreshFailures: number };
}

function makeClient(initial: string | null = 'access-1'): ClientHarness {
	const tokens = { access: initial as string | null, refreshFailures: 0 };
	const fetchMock = vi.fn<typeof fetch>();
	const client = new ApiClient({
		fetchImpl: fetchMock as unknown as typeof fetch,
		getAccessToken: () => tokens.access,
		setAccessToken: (t) => {
			tokens.access = t;
		},
		onRefreshFailure: () => {
			tokens.refreshFailures += 1;
		}
	});
	return { client, fetchMock, tokens };
}

describe('ApiClient.fetch', () => {
	beforeEach(() => {
		vi.useRealTimers();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('attaches Authorization header when token is present', async () => {
		const { client, fetchMock } = makeClient('tok-A');
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch<{ ok: boolean }>('/api/v1/config');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		const headers = init.headers as Headers;
		expect(headers.get('Authorization')).toBe('Bearer tok-A');
	});

	it('adds X-Client-Origin only on mutating requests when clientOrigin is set', async () => {
		const tokens = { access: 'tok' as string | null };
		const fetchMock = vi.fn<typeof fetch>();
		const client = new ApiClient({
			fetchImpl: fetchMock as unknown as typeof fetch,
			getAccessToken: () => tokens.access,
			setAccessToken: () => {},
			onRefreshFailure: () => {},
			clientOrigin: 'tab-xyz'
		});

		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
		await client.fetch('/api/v1/tasks');
		let headers = fetchMock.mock.calls[0][1]!.headers as Headers;
		expect(headers.get('X-Client-Origin')).toBeNull();

		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
		await client.fetch('/api/v1/tasks/1', { method: 'PATCH', body: { title: 'x' } });
		headers = fetchMock.mock.calls[1][1]!.headers as Headers;
		expect(headers.get('X-Client-Origin')).toBe('tab-xyz');
	});

	it('invokes onMutation only after a successful mutating request', async () => {
		const calls: Array<[string, string]> = [];
		const fetchMock = vi.fn<typeof fetch>();
		const client = new ApiClient({
			fetchImpl: fetchMock as unknown as typeof fetch,
			getAccessToken: () => 'tok',
			setAccessToken: () => {},
			onRefreshFailure: () => {},
			onMutation: (path, method) => calls.push([path, method])
		});

		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
		await client.fetch('/api/v1/tasks/1'); // GET → no onMutation
		expect(calls).toHaveLength(0);

		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
		await client.fetch('/api/v1/tasks/1', { method: 'PATCH', body: { title: 'x' } });
		expect(calls).toEqual([['/api/v1/tasks/1', 'PATCH']]);

		// Failed mutation must not fire onMutation.
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ error: { code: 'validation', message: 'bad' } }, 422)
		);
		await expect(
			client.fetch('/api/v1/tasks/1', { method: 'PATCH', body: {} })
		).rejects.toBeInstanceOf(ApiError);
		expect(calls).toHaveLength(1);
	});

	it('parses error envelope into ApiError', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(
			jsonResponse(
				{
					error: { code: 'limit_exceeded', message: 'too many', details: { limit: 30 } }
				},
				422
			)
		);

		await expect(client.fetch('/api/v1/tasks/1/plan', { method: 'POST', body: { state: 'week' } }))
			.rejects.toMatchObject({
				name: 'ApiError',
				code: 'limit_exceeded',
				status: 422,
				details: { limit: 30 }
			});
	});

	it('returns undefined for 204 No Content', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(emptyResponse(204));

		const result = await client.fetch('/api/v1/contexts/1', { method: 'DELETE' });
		expect(result).toBeUndefined();
	});

	it('on 401 auth_expired refreshes token then retries the request once', async () => {
		const { client, fetchMock, tokens } = makeClient('old-access');

		fetchMock
			// initial request → 401 auth_expired
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'auth_expired', message: 'expired' } }, 401)
			)
			// /auth/refresh → new access
			.mockResolvedValueOnce(jsonResponse({ access: 'new-access', refresh: 'new-r' }))
			// retried request → success
			.mockResolvedValueOnce(jsonResponse({ ok: true }));

		const result = await client.fetch<{ ok: boolean }>('/api/v1/inbox');
		expect(result).toEqual({ ok: true });
		expect(tokens.access).toBe('new-access');
		expect(fetchMock).toHaveBeenCalledTimes(3);

		const refreshCall = fetchMock.mock.calls[1];
		expect(refreshCall[0]).toContain('/auth/refresh');
		expect((refreshCall[1] as RequestInit).credentials).toBe('include');

		const retryHeaders = (fetchMock.mock.calls[2][1] as RequestInit).headers as Headers;
		expect(retryHeaders.get('Authorization')).toBe('Bearer new-access');
	});

	it('does not retry when 401 has a different error code', async () => {
		const { client, fetchMock } = makeClient('access');
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ error: { code: 'auth_invalid', message: 'bad token' } }, 401)
		);

		await expect(client.fetch('/api/v1/inbox')).rejects.toMatchObject({
			code: 'auth_invalid',
			status: 401
		});
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('on refresh 401 calls onRefreshFailure and rethrows the original 401', async () => {
		const { client, fetchMock, tokens } = makeClient('old-access');

		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'auth_expired', message: 'expired' } }, 401)
			)
			.mockResolvedValueOnce(emptyResponse(401));

		await expect(client.fetch('/api/v1/inbox')).rejects.toMatchObject({
			code: 'auth_expired',
			status: 401
		});
		expect(tokens.access).toBeNull();
		expect(tokens.refreshFailures).toBe(1);
	});

	it('singleflight: two parallel 401s share one refresh call', async () => {
		const { client, fetchMock, tokens } = makeClient('old-access');

		// inbox-1 → 401, tasks → 401, then refresh, then both retries succeed
		let resolveRefresh!: (value: Response) => void;
		const refreshPromise = new Promise<Response>((res) => {
			resolveRefresh = res;
		});

		fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			const method = init?.method ?? 'GET';
			const headers = init?.headers as Headers | undefined;
			const auth = headers?.get('Authorization') ?? '';

			if (url.endsWith('/auth/refresh') && method === 'POST') {
				return refreshPromise;
			}

			if (url.endsWith('/api/v1/inbox') && auth === 'Bearer old-access') {
				return jsonResponse({ error: { code: 'auth_expired', message: 'e' } }, 401);
			}
			if (url.endsWith('/api/v1/tasks/1') && auth === 'Bearer old-access') {
				return jsonResponse({ error: { code: 'auth_expired', message: 'e' } }, 401);
			}
			if (url.endsWith('/api/v1/inbox') && auth === 'Bearer new-access') {
				return jsonResponse({ kind: 'inbox' });
			}
			if (url.endsWith('/api/v1/tasks/1') && auth === 'Bearer new-access') {
				return jsonResponse({ kind: 'task' });
			}
			throw new Error(`Unexpected request: ${method} ${url} auth=${auth}`);
		});

		const p1 = client.fetch<{ kind: string }>('/api/v1/inbox');
		const p2 = client.fetch<{ kind: string }>('/api/v1/tasks/1');

		// Wait a tick for both initial requests to land and trigger refresh.
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve();

		resolveRefresh(jsonResponse({ access: 'new-access', refresh: 'r' }));

		const [r1, r2] = await Promise.all([p1, p2]);
		expect(r1).toEqual({ kind: 'inbox' });
		expect(r2).toEqual({ kind: 'task' });

		const refreshCalls = fetchMock.mock.calls.filter((c) =>
			String(c[0]).endsWith('/auth/refresh')
		);
		expect(refreshCalls).toHaveLength(1);
		expect(tokens.access).toBe('new-access');
	});

	it('serialises JSON body and query string', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse({ id: 1 }, 201));

		await client.fetch('/api/v1/labels', {
			method: 'POST',
			body: { name: 'x', color: 'red' },
			query: { limit: 50, q: 'a', skip: undefined }
		});

		const [url, init] = fetchMock.mock.calls[0];
		const u = String(url);
		expect(u).toContain('limit=50');
		expect(u).toContain('q=a');
		expect(u).not.toContain('skip=');
		const headers = (init as RequestInit).headers as Headers;
		expect(headers.get('Content-Type')).toBe('application/json');
		expect((init as RequestInit).body).toBe(JSON.stringify({ name: 'x', color: 'red' }));
	});

	it('wraps thrown fetch errors as ApiError(network_error)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockRejectedValueOnce(new TypeError('network down'));

		await expect(client.fetch('/api/v1/config')).rejects.toBeInstanceOf(ApiError);
	});

	it('wraps timeout as ApiError(timeout)', async () => {
		const { client, fetchMock } = makeClient();
		const err = new DOMException('signal timed out', 'TimeoutError');
		fetchMock.mockRejectedValueOnce(err);

		await expect(client.fetch('/api/v1/config')).rejects.toMatchObject({
			name: 'ApiError',
			code: 'timeout',
			message: 'request timed out',
			status: 0
		});
	});

	it('does not retry refresh recursively when refresh itself returns 401', async () => {
		// Sanity: skipRefresh on /auth/refresh prevents a refresh-loop.
		const { client, fetchMock, tokens } = makeClient(null);
		fetchMock.mockResolvedValueOnce(emptyResponse(401));

		await expect(
			client.fetch('/auth/refresh', { method: 'POST', skipAuth: true, skipRefresh: true })
		).rejects.toMatchObject({ status: 401 });

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(tokens.refreshFailures).toBe(0);
	});

	it('appends the canonicalized (sorted) query to the request URL', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks', { query: { b: 2, a: 1, skip: undefined } });

		const url = String(fetchMock.mock.calls[0][0]);
		// buildUrl reuses canonicalizeQuery, so the two agree bit-for-bit.
		expect(url).toBe('/api/v1/tasks?' + canonicalizeQuery({ b: 2, a: 1, skip: undefined }));
		expect(url).toBe('/api/v1/tasks?a=1&b=2');
	});

});

describe('canonicalizeQuery', () => {
	it('returns an empty string for nullish or empty input', () => {
		expect(canonicalizeQuery(undefined)).toBe('');
		expect(canonicalizeQuery(null)).toBe('');
		expect(canonicalizeQuery({})).toBe('');
	});

	it('sorts keys and drops null/undefined values', () => {
		expect(canonicalizeQuery({ b: 2, a: 1, c: undefined, d: null })).toBe('a=1&b=2');
	});

	it('is stable across key insertion order', () => {
		expect(canonicalizeQuery({ q: 'x', limit: 50 })).toBe(canonicalizeQuery({ limit: 50, q: 'x' }));
	});

	it('encodes values the way URLSearchParams does', () => {
		expect(canonicalizeQuery({ q: 'a b', tag: 'c&d' })).toBe('q=a+b&tag=c%26d');
	});

	it('ignores non-object input', () => {
		expect(canonicalizeQuery('foo')).toBe('');
		expect(canonicalizeQuery(42)).toBe('');
	});
});

type BridgeMock = OfflineBridge & {
	isOffline: ReturnType<typeof vi.fn>;
	cacheGet: ReturnType<typeof vi.fn>;
	cachePut: ReturnType<typeof vi.fn>;
	tryEnqueue: ReturnType<typeof vi.fn>;
	noteRequestOutcome: ReturnType<typeof vi.fn>;
};

function makeBridge(overrides: Partial<OfflineBridge> = {}): BridgeMock {
	return {
		isOffline: vi.fn(() => false),
		cacheGet: vi.fn(async () => null),
		cachePut: vi.fn(async () => {}),
		tryEnqueue: vi.fn(async () => null),
		noteRequestOutcome: vi.fn(),
		...overrides
	} as BridgeMock;
}

function makeOfflineClient(bridge: OfflineBridge, initial: string | null = 'access-1') {
	const tokens = { access: initial as string | null, refreshFailures: 0 };
	const fetchMock = vi.fn<typeof fetch>();
	const client = new ApiClient({
		fetchImpl: fetchMock as unknown as typeof fetch,
		getAccessToken: () => tokens.access,
		setAccessToken: (t) => {
			tokens.access = t;
		},
		onRefreshFailure: () => {
			tokens.refreshFailures += 1;
		},
		offline: bridge
	});
	return { client, fetchMock, tokens };
}

// Flush the microtask/macrotask queue so a fire-and-forget background probe runs.
function flush(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('ApiClient.fetch offline integration (GET path)', () => {
	beforeEach(() => {
		vi.useRealTimers();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('writes a successful GET through to the cache and notes online', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ id: 1 }));

		const result = await client.fetch('/api/v1/tasks/1');

		expect(result).toEqual({ id: 1 });
		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(true);
		expect(bridge.cachePut).toHaveBeenCalledWith('/api/v1/tasks/1', undefined, { id: 1 });
	});

	it('passes the query through to cachePut on write-through', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks', { query: { limit: 50 } });

		expect(bridge.cachePut).toHaveBeenCalledWith('/api/v1/tasks', { limit: 50 }, { ok: true });
	});

	it('does not cache-write a mutation but still notes online on success', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks/1', { method: 'PATCH', body: { title: 'x' } });

		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(true);
		expect(bridge.cachePut).not.toHaveBeenCalled();
	});

	it('serves a stale cache hit when a GET fails with network_error (status 0)', async () => {
		const bridge = makeBridge({
			cacheGet: vi.fn(async () => ({ payload: { id: 7, cached: true }, storedAt: 't' }))
		});
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		const result = await client.fetch('/api/v1/tasks/7');

		expect(result).toEqual({ id: 7, cached: true });
		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(false);
		expect(bridge.cacheGet).toHaveBeenCalledWith('/api/v1/tasks/7', undefined);
	});

	it('rethrows the network error when a failed GET has no cache hit', async () => {
		const bridge = makeBridge(); // cacheGet → null
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await expect(client.fetch('/api/v1/tasks/7')).rejects.toMatchObject({
			code: 'network_error',
			status: 0
		});
		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(false);
	});

	it('serves cache-first when offline and refreshes via a background network probe', async () => {
		const bridge = makeBridge({
			isOffline: vi.fn(() => true),
			cacheGet: vi.fn(async () => ({ payload: { id: 9, stale: true }, storedAt: 't' }))
		});
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ id: 9, fresh: true }));

		const result = await client.fetch('/api/v1/tasks/9');
		// Immediate return from cache — no waiting on the network.
		expect(result).toEqual({ id: 9, stale: true });

		// The background probe then hits the network, refreshes cache, flips online.
		await flush();
		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(bridge.cachePut).toHaveBeenCalledWith('/api/v1/tasks/9', undefined, {
			id: 9,
			fresh: true
		});
		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(true);
	});

	it('when offline with no cache hit, falls through to the network', async () => {
		const bridge = makeBridge({ isOffline: vi.fn(() => true) }); // cacheGet → null
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ live: true }));

		const result = await client.fetch('/api/v1/tasks/9');

		expect(result).toEqual({ live: true });
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('skipOffline bypasses the cache-first path and hits the network even when offline', async () => {
		const bridge = makeBridge({
			isOffline: vi.fn(() => true),
			cacheGet: vi.fn(async () => ({ payload: { cached: true }, storedAt: 't' }))
		});
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ fresh: true }));

		const result = await client.fetch('/api/v1/tasks', { skipOffline: true });

		expect(result).toEqual({ fresh: true });
		expect(bridge.cacheGet).not.toHaveBeenCalled();
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('skipOffline does not serve stale on a network failure — it rethrows', async () => {
		const bridge = makeBridge({
			cacheGet: vi.fn(async () => ({ payload: { cached: true }, storedAt: 't' }))
		});
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await expect(client.fetch('/api/v1/tasks', { skipOffline: true })).rejects.toMatchObject({
			status: 0
		});
		expect(bridge.cacheGet).not.toHaveBeenCalled();
	});

	it('does not flip offline on a 4xx/5xx server error (real status, not 0)', async () => {
		const bridge = makeBridge({
			cacheGet: vi.fn(async () => ({ payload: { cached: true }, storedAt: 't' }))
		});
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(
			jsonResponse({ error: { code: 'not_found', message: 'nope' } }, 404)
		);

		await expect(client.fetch('/api/v1/tasks/7')).rejects.toMatchObject({ status: 404 });
		expect(bridge.noteRequestOutcome).not.toHaveBeenCalledWith(false);
		expect(bridge.cacheGet).not.toHaveBeenCalled();
	});
});

describe('ApiClient.fetch offline integration (mutation path)', () => {
	beforeEach(() => {
		vi.useRealTimers();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('attaches one Idempotency-Key to a mutation and reuses it across a 401 retry', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge, 'old-access');
		fetchMock
			// initial POST → 401 auth_expired
			.mockResolvedValueOnce(
				jsonResponse({ error: { code: 'auth_expired', message: 'e' } }, 401)
			)
			// /auth/refresh → new access
			.mockResolvedValueOnce(jsonResponse({ access: 'new-access', refresh: 'r' }))
			// retried POST → success
			.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks/1/complete', { method: 'POST' });

		const firstHeaders = fetchMock.mock.calls[0][1]!.headers as Headers;
		const retryHeaders = fetchMock.mock.calls[2][1]!.headers as Headers;
		const key = firstHeaders.get('Idempotency-Key');
		expect(key).toBeTruthy();
		// The internal retry reuses the exact same key (one key per fetch() call).
		expect(retryHeaders.get('Idempotency-Key')).toBe(key);
		// The refresh request itself carries no idempotency key.
		const refreshHeaders = fetchMock.mock.calls[1][1]!.headers as Headers;
		expect(refreshHeaders.get('Idempotency-Key')).toBeNull();
	});

	it('honors an explicit init.idempotencyKey (replay reuse)', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks/1/complete', {
			method: 'POST',
			idempotencyKey: 'replay-key'
		});

		const headers = fetchMock.mock.calls[0][1]!.headers as Headers;
		expect(headers.get('Idempotency-Key')).toBe('replay-key');
	});

	it('does not attach an Idempotency-Key to GET requests', async () => {
		const bridge = makeBridge();
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks/1');

		const headers = fetchMock.mock.calls[0][1]!.headers as Headers;
		expect(headers.get('Idempotency-Key')).toBeNull();
	});

	it('enqueues a mutation that failed with network_error and returns the synthesized response', async () => {
		const synthesized = { id: 1, status: 'completed' };
		const bridge = makeBridge({ tryEnqueue: vi.fn(async () => ({ response: synthesized })) });
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		const result = await client.fetch('/api/v1/tasks/1/complete', { method: 'POST' });

		expect(result).toEqual(synthesized);
		expect(bridge.tryEnqueue).toHaveBeenCalledWith(
			'/api/v1/tasks/1/complete',
			'POST',
			undefined,
			expect.any(String)
		);
		expect(bridge.noteRequestOutcome).toHaveBeenCalledWith(false);
	});

	it('passes the request body through to tryEnqueue', async () => {
		const bridge = makeBridge({ tryEnqueue: vi.fn(async () => ({ response: { id: -1 } })) });
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await client.fetch('/api/v1/inbox/tasks', { method: 'POST', body: { title: 'x' } });

		expect(bridge.tryEnqueue).toHaveBeenCalledWith(
			'/api/v1/inbox/tasks',
			'POST',
			{ title: 'x' },
			expect.any(String)
		);
	});

	it('enqueues under the SAME Idempotency-Key it sent to the server (§6.3 lost-response replay)', async () => {
		const bridge = makeBridge({ tryEnqueue: vi.fn(async () => ({ response: { id: 1 } })) });
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await client.fetch('/api/v1/tasks/1/complete', { method: 'POST' });

		// The key attached to the (failed) network attempt...
		const sentKey = (fetchMock.mock.calls[0][1]!.headers as Headers).get('Idempotency-Key');
		expect(sentKey).toBeTruthy();
		// ...must be handed to the outbox verbatim, so replay resends it and the
		// backend recognises the lost-response retry instead of re-executing.
		expect(bridge.tryEnqueue.mock.calls[0][3]).toBe(sentKey);
	});

	it('throws offline_unsupported when tryEnqueue declines the op (null)', async () => {
		const bridge = makeBridge(); // tryEnqueue → null
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await expect(
			client.fetch('/api/v1/tasks/1/move', { method: 'POST', body: {} })
		).rejects.toMatchObject({ code: 'offline_unsupported', status: 0 });
		expect(bridge.tryEnqueue).toHaveBeenCalled();
	});

	it('does not fire onMutation for a queued (offline) mutation', async () => {
		const calls: Array<[string, string]> = [];
		const bridge = makeBridge({ tryEnqueue: vi.fn(async () => ({ response: { ok: true } })) });
		const fetchMock = vi.fn<typeof fetch>();
		const client = new ApiClient({
			fetchImpl: fetchMock as unknown as typeof fetch,
			getAccessToken: () => 'tok',
			setAccessToken: () => {},
			onRefreshFailure: () => {},
			onMutation: (path, method) => calls.push([path, method]),
			offline: bridge
		});
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await client.fetch('/api/v1/tasks/1/complete', { method: 'POST' });

		expect(calls).toHaveLength(0);
	});

	it('skipOffline mutation does not enqueue — it rethrows the network error (replay reissue)', async () => {
		const bridge = makeBridge({ tryEnqueue: vi.fn(async () => ({ response: {} })) });
		const { client, fetchMock } = makeOfflineClient(bridge);
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		await expect(
			client.fetch('/api/v1/tasks/1/complete', { method: 'POST', skipOffline: true })
		).rejects.toMatchObject({ status: 0 });
		expect(bridge.tryEnqueue).not.toHaveBeenCalled();
	});
});

describe('ApiClient.fetch without an offline bridge (regression)', () => {
	beforeEach(() => {
		vi.useRealTimers();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('behaves identically when no offline bridge is configured', async () => {
		const { client, fetchMock } = makeClient();

		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));
		await expect(client.fetch('/api/v1/config')).resolves.toEqual({ ok: true });

		// A network failure still surfaces as ApiError — never silently served from a cache.
		fetchMock.mockRejectedValueOnce(new TypeError('down'));
		await expect(client.fetch('/api/v1/config')).rejects.toMatchObject({
			code: 'network_error',
			status: 0
		});
		expect(fetchMock).toHaveBeenCalledTimes(2);
	});

	it('does not attach an Idempotency-Key to mutations when no offline bridge is configured', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true }));

		await client.fetch('/api/v1/tasks/1/complete', { method: 'POST' });

		const headers = fetchMock.mock.calls[0][1]!.headers as Headers;
		expect(headers.get('Idempotency-Key')).toBeNull();
	});
});
