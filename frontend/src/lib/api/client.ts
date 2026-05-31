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
}

export type QueryValue = string | number | boolean | undefined | null;

interface ApiFetchInit extends Omit<RequestInit, 'body'> {
	body?: unknown;
	query?: Record<string, QueryValue> | object;
	skipAuth?: boolean;
	skipRefresh?: boolean;
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
	}

	private static readonly mutatingMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

	async fetch<T>(path: string, init: ApiFetchInit = {}): Promise<T> {
		const url = this.buildUrl(path, init.query);
		const method = (init.method ?? 'GET').toUpperCase();
		const reqBody = init.body != null
			? (typeof init.body === 'string' ? init.body : JSON.stringify(init.body))
			: null;
		const start = performance.now();

		try {
			const response = await this.doRequest(url, init, /*isRetry*/ false);
			const result = await this.parseResponse<T>(response);
			this.emitLog(method, url, response.status, start, reqBody, result, null);
			if (this.onMutation && ApiClient.mutatingMethods.has(method)) {
				this.onMutation(path, method);
			}
			return result;
		} catch (err) {
			const status = err instanceof ApiError ? err.status : null;
			const errMsg = err instanceof Error ? err.message : String(err);
			this.emitLog(method, url, status, start, reqBody, null, errMsg);
			throw err;
		}
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
		if (!query) return base;
		const params = new URLSearchParams();
		for (const [key, value] of Object.entries(query as Record<string, unknown>)) {
			if (value === undefined || value === null) continue;
			params.append(key, String(value));
		}
		const qs = params.toString();
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
		let response: Response;
		try {
			response = await this.fetchImpl(this.baseUrl + '/auth/refresh', {
				method: 'POST',
				credentials: 'include'
			});
		} catch {
			// Network error: leave session intact; caller will see the original 401 and surface it.
			return null;
		}
		if (!response.ok) {
			// Only treat actual auth rejection as a forced logout; transient 5xx must not log the user out.
			if (response.status === 401 || response.status === 403) {
				this.setAccessToken(null);
				this.onRefreshFailure();
			}
			return null;
		}
		const data = (await response.json()) as RefreshResponseBody;
		this.setAccessToken(data.access);
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
