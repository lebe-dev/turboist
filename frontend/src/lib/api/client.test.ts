import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiClient } from './client';
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

});
