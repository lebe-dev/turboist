import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import type { Peer } from '$lib/api/types';

// Capture eventsClient.on handlers so a test can fire a federation-origin SSE
// scope event and assert PeersTable reloads its pendingDelivery (US-3.2 AC4).
const sseHandlers: Record<string, Array<(scope: string) => void>> = {};
vi.mock('$lib/realtime/events.svelte', () => ({
	eventsClient: {
		on(scope: string, handler: (scope: string) => void) {
			(sseHandlers[scope] ??= []).push(handler);
			return () => {
				sseHandlers[scope] = (sseHandlers[scope] ?? []).filter((h) => h !== handler);
			};
		}
	}
}));

function fireSSE(scope: string): void {
	for (const h of sseHandlers[scope] ?? []) h(scope);
}

// Import after the mock so the component picks up the mocked eventsClient.
const { default: PeersTable } = await import('./PeersTable.svelte');

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

interface CapturedRequest {
	url: string;
	method: string;
}

function peer(over: Partial<Peer> = {}): Peer {
	return {
		instanceUrl: 'https://bob.example',
		displayName: "Bob's Box",
		permissions: 'write',
		status: 'active',
		lastSentHlc: '0000000000000-00000-node',
		lastContactAt: '2026-06-01T00:00:00.000Z',
		joinedAt: '2026-01-01T00:00:00.000Z',
		pendingDelivery: 0,
		keyMismatchAt: '',
		...over
	};
}

function makeFetchMock(list: Peer[], captured: CapturedRequest[]): typeof fetch {
	return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === 'string' ? input : input.toString();
		const method = (init?.method ?? 'GET').toUpperCase();
		captured.push({ url, method });
		if (url.endsWith('/api/v1/projects/7/federation/peers') && method === 'GET') {
			return jsonResponse(list);
		}
		return new Response(null, { status: 204 });
	}) as unknown as typeof fetch;
}

function setupAuth(fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled: false };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

afterEach(async () => {
	// The revoke / trust-key tests open a ConfirmDestructiveDialog (bits-ui), which
	// on unmount schedules a deferred body-scroll-lock reset (a ~24ms setTimeout).
	// Unmount now and let that timer fire while `document` still exists — otherwise
	// it runs after jsdom is torn down and throws "document is not defined" as an
	// unhandled error (flaky, surfacing only in the full-suite run).
	cleanup();
	await new Promise((resolve) => setTimeout(resolve, 50));
	vi.restoreAllMocks();
});

describe('PeersTable (Federation v1 F1.4)', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
		for (const k of Object.keys(sseHandlers)) delete sseHandlers[k];
	});

	it('renders a row per peer as "displayName @ instanceUrl" (US-1.4 AC1, AC2)', async () => {
		const list = [
			peer({ instanceUrl: 'https://bob.example', displayName: "Bob's Box" }),
			peer({ instanceUrl: 'https://carol.example', displayName: 'Carol', status: 'stale' })
		];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		// US-1.4 AC2: each row shows "display_name @ instance".
		expect(await screen.findByText(/Bob's Box @ https:\/\/bob\.example/)).toBeTruthy();
		expect(screen.getByText(/Carol @ https:\/\/carol\.example/)).toBeTruthy();
	});

	it('renders the derived status label for each peer (US-1.4 AC1, AC3)', async () => {
		const list = [
			peer({ instanceUrl: 'https://a.example', displayName: 'A', status: 'active' }),
			peer({ instanceUrl: 'https://s.example', displayName: 'S', status: 'stale' }),
			peer({ instanceUrl: 'https://p.example', displayName: 'P', status: 'paused' }),
			peer({ instanceUrl: 'https://r.example', displayName: 'R', status: 'revoked' })
		];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		expect(await screen.findByText(/^Active$/i)).toBeTruthy();
		expect(screen.getByText(/^Stale$/i)).toBeTruthy();
		expect(screen.getByText(/^Paused$/i)).toBeTruthy();
		expect(screen.getByText(/^Revoked$/i)).toBeTruthy();
	});

	it('shows the pending-delivery metric for each peer (US-1.4 AC4 partial)', async () => {
		const list = [peer({ pendingDelivery: 0 })];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		// The pending metric is rendered (0 until the outbox lands).
		const row = (await screen.findByText(/Bob's Box @/)).closest('[data-peer-row]') as HTMLElement;
		expect(row.textContent).toMatch(/0/);
	});

	it('renders an empty-state message when there are no peers', async () => {
		const fetchMock = makeFetchMock([], captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));
		expect(await screen.findByText(/no peers/i)).toBeTruthy();
	});

	it('shows a Pause action for an active peer and POSTs the peer URL in the body (US-6.1 AC1)', async () => {
		const list = [peer({ instanceUrl: 'https://bob.example', displayName: 'Bob', status: 'active' })];
		const requests: Array<{ url: string; method: string; body: string }> = [];
		const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === 'string' ? input : input.toString();
			const method = (init?.method ?? 'GET').toUpperCase();
			requests.push({ url, method, body: String(init?.body ?? '') });
			if (url.endsWith('/api/v1/projects/7/federation/peers') && method === 'GET') {
				return jsonResponse(list);
			}
			return new Response(null, { status: 204 });
		}) as unknown as typeof fetch;
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		const pauseBtn = await screen.findByRole('button', { name: /pause/i });
		await fireEvent.click(pauseBtn);

		await waitFor(() =>
			expect(
				requests.some(
					(r) => r.method === 'POST' && r.url.endsWith('/api/v1/projects/7/federation/peers/pause')
				)
			).toBe(true)
		);
		const pauseReq = requests.find((r) => r.url.endsWith('/peers/pause'))!;
		expect(JSON.parse(pauseReq.body)).toEqual({ instanceUrl: 'https://bob.example' });
	});

	it('shows a Resume action for a paused peer and POSTs to the resume route (US-6.1 AC2/AC3)', async () => {
		const list = [peer({ instanceUrl: 'https://bob.example', displayName: 'Bob', status: 'paused' })];
		const requests: Array<{ url: string; method: string }> = [];
		const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === 'string' ? input : input.toString();
			const method = (init?.method ?? 'GET').toUpperCase();
			requests.push({ url, method });
			if (url.endsWith('/api/v1/projects/7/federation/peers') && method === 'GET') {
				return jsonResponse(list);
			}
			return new Response(null, { status: 204 });
		}) as unknown as typeof fetch;
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		// A paused peer offers Resume, not Pause (US-6.1 AC3).
		const resumeBtn = await screen.findByRole('button', { name: /resume/i });
		expect(screen.queryByRole('button', { name: /^pause$/i })).toBeNull();
		await fireEvent.click(resumeBtn);

		await waitFor(() =>
			expect(
				requests.some(
					(r) => r.method === 'POST' && r.url.endsWith('/api/v1/projects/7/federation/peers/resume')
				)
			).toBe(true)
		);
	});

	it('offers neither Pause nor Resume for a revoked peer (terminal state)', async () => {
		const list = [peer({ instanceUrl: 'https://r.example', displayName: 'R', status: 'revoked' })];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		expect(await screen.findByText(/R @ https:\/\/r\.example/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: /pause/i })).toBeNull();
		expect(screen.queryByRole('button', { name: /resume/i })).toBeNull();
	});

	it('offers no Revoke for an already-revoked peer (terminal state, Federation v1 F5.4)', async () => {
		const list = [peer({ instanceUrl: 'https://r.example', displayName: 'R', status: 'revoked' })];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		expect(await screen.findByText(/R @ https:\/\/r\.example/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: /^revoke$/i })).toBeNull();
	});

	it('renders the "Left" status and offers no controls for a peer that left (Federation v1 F5.5, US-6.3 AC2)', async () => {
		const list = [peer({ instanceUrl: 'https://l.example', displayName: 'L', status: 'left' })];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		expect(await screen.findByText(/L @ https:\/\/l\.example/)).toBeTruthy();
		// Owner UI shows the distinct "Left" status (US-6.3 AC2).
		expect(screen.getByText(/^Left$/i)).toBeTruthy();
		// A left peer is terminal — no pause/resume/revoke controls.
		expect(screen.queryByRole('button', { name: /pause/i })).toBeNull();
		expect(screen.queryByRole('button', { name: /resume/i })).toBeNull();
		expect(screen.queryByRole('button', { name: /^revoke$/i })).toBeNull();
	});

	it('confirms then DELETEs the peer URL in the body for an active peer (US-6.2 AC1)', async () => {
		const list = [peer({ instanceUrl: 'https://bob.example', displayName: 'Bob', status: 'active' })];
		const requests: Array<{ url: string; method: string; body: string }> = [];
		const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === 'string' ? input : input.toString();
			const method = (init?.method ?? 'GET').toUpperCase();
			requests.push({ url, method, body: String(init?.body ?? '') });
			if (url.endsWith('/api/v1/projects/7/federation/peers') && method === 'GET') {
				return jsonResponse(list);
			}
			return new Response(null, { status: 204 });
		}) as unknown as typeof fetch;
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		// The peer row offers a Revoke action; clicking it opens the irreversible
		// confirm dialog rather than firing the DELETE immediately (US-6.2 AC5).
		const revokeBtn = await screen.findByRole('button', { name: /^revoke$/i });
		await fireEvent.click(revokeBtn);

		// No DELETE yet — only after confirming.
		expect(requests.some((r) => r.method === 'DELETE')).toBe(false);

		// The confirm dialog now shows a destructive confirm button labelled Revoke.
		const confirmButtons = await screen.findAllByRole('button', { name: /^revoke$/i });
		// Click the one inside the dialog (the last rendered Revoke control).
		await fireEvent.click(confirmButtons[confirmButtons.length - 1]);

		await waitFor(() =>
			expect(
				requests.some(
					(r) => r.method === 'DELETE' && r.url.endsWith('/api/v1/projects/7/federation/peers')
				)
			).toBe(true)
		);
		const revokeReq = requests.find((r) => r.method === 'DELETE')!;
		expect(JSON.parse(revokeReq.body)).toEqual({ instanceUrl: 'https://bob.example' });
	});

	it('renders the key-change incident alert + a Trust new key action for a peer with keyMismatchAt (Federation v1 F5.6b, US-6.4 AC2)', async () => {
		const list = [
			peer({
				instanceUrl: 'https://bob.example',
				displayName: 'Bob',
				status: 'active',
				keyMismatchAt: '2026-06-03T10:00:00.000Z'
			})
		];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		// US-6.4 AC2: the incident alert names the peer and flags a possible key
		// rotation / compromise, and a "Trust new key" action is offered.
		expect(await screen.findByText(/signature failed/i)).toBeTruthy();
		expect(screen.getByRole('button', { name: /trust new key/i })).toBeTruthy();
		// A healthy peer (empty keyMismatchAt) renders no incident alert.
	});

	it('does NOT render the incident alert for a healthy peer (US-6.4 AC2)', async () => {
		const list = [peer({ instanceUrl: 'https://ok.example', displayName: 'OK', status: 'active' })];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		expect(await screen.findByText(/OK @ https:\/\/ok\.example/)).toBeTruthy();
		expect(screen.queryByText(/signature failed/i)).toBeNull();
		expect(screen.queryByRole('button', { name: /trust new key/i })).toBeNull();
	});

	it('confirms then POSTs the trust-key route for a key-changed peer (Federation v1 F5.6b, US-6.4 AC3)', async () => {
		const list = [
			peer({
				instanceUrl: 'https://bob.example',
				displayName: 'Bob',
				status: 'active',
				keyMismatchAt: '2026-06-03T10:00:00.000Z'
			})
		];
		const requests: Array<{ url: string; method: string; body: string }> = [];
		const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === 'string' ? input : input.toString();
			const method = (init?.method ?? 'GET').toUpperCase();
			requests.push({ url, method, body: String(init?.body ?? '') });
			if (url.endsWith('/api/v1/projects/7/federation/peers') && method === 'GET') {
				return jsonResponse(list);
			}
			return new Response(null, { status: 204 });
		}) as unknown as typeof fetch;
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		// Clicking "Trust new key" opens a confirm dialog (a deliberate security
		// action); the POST fires only after confirming.
		const trustBtn = await screen.findByRole('button', { name: /trust new key/i });
		await fireEvent.click(trustBtn);
		expect(requests.some((r) => r.url.endsWith('/peers/trust-key'))).toBe(false);

		const confirmButtons = await screen.findAllByRole('button', { name: /trust new key/i });
		await fireEvent.click(confirmButtons[confirmButtons.length - 1]);

		await waitFor(() =>
			expect(
				requests.some(
					(r) =>
						r.method === 'POST' &&
						r.url.endsWith('/api/v1/projects/7/federation/peers/trust-key')
				)
			).toBe(true)
		);
		const trustReq = requests.find((r) => r.url.endsWith('/peers/trust-key'))!;
		expect(JSON.parse(trustReq.body)).toEqual({ instanceUrl: 'https://bob.example' });
	});

	it('reloads the peers list (and pendingDelivery) on a federation-origin SSE event (US-3.2 AC4)', async () => {
		// First fetch returns 2 pending; after the SSE event a fresh fetch returns 0.
		let pending = 2;
		const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = typeof input === 'string' ? input : input.toString();
			const method = (init?.method ?? 'GET').toUpperCase();
			captured.push({ url, method });
			if (url.endsWith('/api/v1/projects/7/federation/peers')) {
				return jsonResponse([peer({ pendingDelivery: pending })]);
			}
			return new Response(null, { status: 204 });
		}) as unknown as typeof fetch;
		setupAuth(fetchMock);
		render(PeersTable, { projectId: 7 });

		await waitFor(() => expect(captured.filter((r) => r.method === 'GET').length).toBe(1));
		// 2 events pending delivery before the remote change.
		expect(await screen.findByText(/2 pending delivery/)).toBeTruthy();

		// A remote (federation-origin) change arrives → reload, now 0 pending.
		pending = 0;
		fireSSE('projects');

		await waitFor(() => expect(captured.filter((r) => r.method === 'GET').length).toBe(2));
		await waitFor(() => expect(screen.getByText(/0 pending delivery/)).toBeTruthy());
	});
});
