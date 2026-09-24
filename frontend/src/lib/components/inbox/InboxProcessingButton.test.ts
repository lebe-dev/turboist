import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { toastMock } = vi.hoisted(() => ({
	toastMock: { success: vi.fn(), error: vi.fn(), info: vi.fn() }
}));

vi.mock('svelte-sonner', () => ({
	toast: toastMock
}));

import { createAuthStore } from '$lib/auth/store.svelte';
import type { InboxProcessingStatus } from '$lib/api/types';
import InboxProcessingButton from './InboxProcessingButton.svelte';

function status(overrides: Partial<InboxProcessingStatus> = {}): InboxProcessingStatus {
	return {
		enabled: true,
		model: 'openai/gpt-4.1-mini',
		apiHost: 'openrouter.ai',
		interval: '3m',
		batchLimit: 10,
		running: false,
		pendingCount: 2,
		undecidedCount: 0,
		lastRunAt: null,
		lastRunSummary: null,
		lastError: null,
		backoffUntil: null,
		paused: true,
		defaultPrompt: '',
		...overrides
	};
}

function json(body: unknown, code = 200): Response {
	return new Response(JSON.stringify(body), {
		status: code,
		headers: { 'Content-Type': 'application/json' }
	});
}

interface MockOptions {
	statuses: InboxProcessingStatus[];
	runResponse?: () => Response;
}

function makeFetchMock(calls: string[], opts: MockOptions): typeof fetch {
	let statusIndex = 0;
	return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === 'string' ? input : input.toString();
		const method = (init?.method ?? 'GET').toUpperCase();
		calls.push(`${method} ${url.replace(/^https?:\/\/[^/]+/, '')}`);
		if (method === 'POST' && url.endsWith('/api/v1/inbox/processing/run')) {
			return opts.runResponse ? opts.runResponse() : json({ running: true }, 202);
		}
		if (method === 'GET' && url.endsWith('/api/v1/inbox/processing')) {
			const s = opts.statuses[Math.min(statusIndex, opts.statuses.length - 1)];
			statusIndex++;
			return json(s);
		}
		return new Response(null, { status: 404 });
	}) as unknown as typeof fetch;
}

function setupAuth(fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled: false };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

function button(): HTMLButtonElement {
	return screen.getByRole('button', { name: /sort with ai|разобрать с ии|sorting|разбираю/i }) as HTMLButtonElement;
}

afterEach(() => {
	vi.restoreAllMocks();
	toastMock.success.mockReset();
	toastMock.error.mockReset();
	toastMock.info.mockReset();
});

describe('InboxProcessingButton', () => {
	let calls: string[];

	beforeEach(() => {
		calls = [];
	});

	it('starts a run and reports its summary once it finishes, even while paused', async () => {
		setupAuth(
			makeFetchMock(calls, {
				statuses: [
					status({ running: true }),
					status({ running: false, lastRunSummary: { sorted: 2, kept: 1, failed: 0 } })
				]
			})
		);
		render(InboxProcessingButton, { props: { pollIntervalMs: 5 } });

		await fireEvent.click(button());

		await waitFor(() => expect(toastMock.success).toHaveBeenCalledTimes(1));
		expect(calls[0]).toBe('POST /api/v1/inbox/processing/run');
		expect(calls.filter((c) => c === 'GET /api/v1/inbox/processing')).toHaveLength(2);
		expect(toastMock.success.mock.calls[0][0]).toMatch(/2/);
		expect(button().disabled).toBe(false);
	});

	it('reports the provider error of a failed run', async () => {
		setupAuth(
			makeFetchMock(calls, {
				statuses: [status({ running: false, lastError: 'provider unavailable' })]
			})
		);
		render(InboxProcessingButton, { props: { pollIntervalMs: 5 } });

		await fireEvent.click(button());

		await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith('provider unavailable'));
	});

	it('says there is nothing to sort when every task was already considered', async () => {
		setupAuth(
			makeFetchMock(calls, {
				statuses: [status()],
				runResponse: () =>
					json(
						{
							error: {
								code: 'inbox_processing_nothing_pending',
								message: 'nothing to process',
								details: {}
							}
						},
						409
					)
			})
		);
		render(InboxProcessingButton, { props: { pollIntervalMs: 5 } });

		await fireEvent.click(button());

		await waitFor(() => expect(toastMock.info).toHaveBeenCalledTimes(1));
		expect(calls).toEqual(['POST /api/v1/inbox/processing/run']);
		expect(button().disabled).toBe(false);
	});
});
