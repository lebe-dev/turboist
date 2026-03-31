import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock logger before imports
vi.mock('$lib/stores/logger', () => ({
	logger: {
		log: vi.fn(),
		warn: vi.fn(),
		error: vi.fn()
	}
}));

// Minimal WebSocket mock
class MockWebSocket {
	static CONNECTING = 0;
	static OPEN = 1;
	static CLOSING = 2;
	static CLOSED = 3;

	static instances: MockWebSocket[] = [];

	readyState = MockWebSocket.CONNECTING;
	onopen: (() => void) | null = null;
	onclose: ((event: { code: number; reason: string }) => void) | null = null;
	onmessage: ((event: { data: string }) => void) | null = null;
	onerror: (() => void) | null = null;
	sent: string[] = [];

	constructor(public url: string) {
		MockWebSocket.instances.push(this);
	}

	send(data: string) {
		this.sent.push(data);
	}

	close() {
		this.readyState = MockWebSocket.CLOSED;
		this.onclose?.({ code: 1000, reason: '' });
	}

	// Test helpers
	simulateOpen() {
		this.readyState = MockWebSocket.OPEN;
		this.onopen?.();
	}

	simulateMessage(data: unknown) {
		this.onmessage?.({ data: JSON.stringify(data) });
	}

	simulateClose(code = 1006, reason = '') {
		this.readyState = MockWebSocket.CLOSED;
		this.onclose?.({ code, reason });
	}
}

beforeEach(() => {
	MockWebSocket.instances = [];
	vi.stubGlobal('WebSocket', MockWebSocket);
	vi.stubGlobal('location', { protocol: 'https:', host: 'example.com' });
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
	// Reset module cache so each test gets a fresh singleton
	vi.resetModules();
});

async function loadClient() {
	const mod = await import('./client.svelte');
	return mod.wsClient;
}

describe('wsClient', () => {
	it('connects via WebSocket', async () => {
		const client = await loadClient();
		client.connect();

		expect(MockWebSocket.instances).toHaveLength(1);
		expect(MockWebSocket.instances[0].url).toBe('wss://example.com/api/ws');
	});

	it('uses ws: for http:', async () => {
		vi.stubGlobal('location', { protocol: 'http:', host: 'localhost:8080' });
		const client = await loadClient();
		client.connect();

		expect(MockWebSocket.instances[0].url).toBe('ws://localhost:8080/api/ws');
	});

	it('sets connected=true on open', async () => {
		const client = await loadClient();
		client.connect();
		expect(client.connected).toBe(false);

		MockWebSocket.instances[0].simulateOpen();
		expect(client.connected).toBe(true);
	});

	it('sets connected=false on close', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();
		expect(client.connected).toBe(true);

		MockWebSocket.instances[0].simulateClose();
		expect(client.connected).toBe(false);
	});

	it('routes snapshot messages to handlers', async () => {
		const client = await loadClient();
		const handler = vi.fn();
		client.onMessage('snapshot', 'tasks', handler);
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		const data = { tasks: [], meta: {} };
		MockWebSocket.instances[0].simulateMessage({
			type: 'snapshot',
			channel: 'tasks',
			data
		});

		expect(handler).toHaveBeenCalledWith(data, undefined);
	});

	it('routes delta messages to handlers', async () => {
		const client = await loadClient();
		const handler = vi.fn();
		client.onMessage('delta', 'planning', handler);
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		const data = { backlog: [], weekly: [] };
		MockWebSocket.instances[0].simulateMessage({
			type: 'delta',
			channel: 'planning',
			data
		});

		expect(handler).toHaveBeenCalledWith(data, undefined);
	});

	it('responds to ping with pong', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		MockWebSocket.instances[0].simulateMessage({ type: 'ping' });

		const sent = MockWebSocket.instances[0].sent;
		expect(sent).toHaveLength(1);
		expect(JSON.parse(sent[0])).toEqual({ type: 'pong' });
	});

	it('unregisters handler on cleanup', async () => {
		const client = await loadClient();
		const handler = vi.fn();
		const cleanup = client.onMessage('snapshot', 'tasks', handler);
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		cleanup();

		MockWebSocket.instances[0].simulateMessage({
			type: 'snapshot',
			channel: 'tasks',
			data: {}
		});
		expect(handler).not.toHaveBeenCalled();
	});

	it('sends subscribe message', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		client.subscribe('tasks', { view: 'today', context: 'work' });

		const sent = MockWebSocket.instances[0].sent;
		const msg = JSON.parse(sent[0]);
		expect(msg.type).toBe('subscribe');
		expect(msg.channel).toBe('tasks');
		expect(msg.view).toBe('today');
		expect(msg.context).toBe('work');
		expect(msg.seq).toBeGreaterThan(0);
	});

	it('resubscribes on reconnect', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();
		client.subscribe('tasks', { view: 'today' });

		// Simulate disconnect + reconnect
		MockWebSocket.instances[0].simulateClose();
		vi.advanceTimersByTime(1000);

		// New connection opens
		expect(MockWebSocket.instances).toHaveLength(2);
		MockWebSocket.instances[1].simulateOpen();

		// Should have resubscribed with a new (higher) seq
		const sent = MockWebSocket.instances[1].sent;
		expect(sent.length).toBeGreaterThanOrEqual(1);
		const msg = JSON.parse(sent[0]);
		expect(msg.type).toBe('subscribe');
		expect(msg.channel).toBe('tasks');
		expect(msg.view).toBe('today');
		expect(msg.seq).toBeGreaterThan(0);
	});

	it('notifies state change listeners', async () => {
		const client = await loadClient();
		const listener = vi.fn();
		client.onStateChange(listener);
		client.connect();

		MockWebSocket.instances[0].simulateOpen();
		expect(listener).toHaveBeenCalledWith(true);

		MockWebSocket.instances[0].simulateClose();
		expect(listener).toHaveBeenCalledWith(false);
	});

	it('removes state change listener on cleanup', async () => {
		const client = await loadClient();
		const listener = vi.fn();
		const cleanup = client.onStateChange(listener);
		client.connect();

		cleanup();
		MockWebSocket.instances[0].simulateOpen();
		expect(listener).not.toHaveBeenCalled();
	});

	it('disconnect prevents reconnect', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		client.disconnect();
		expect(client.connected).toBe(false);

		vi.advanceTimersByTime(60000);
		// No new connection after intentional disconnect
		expect(MockWebSocket.instances).toHaveLength(1);
	});

	it('does not connect twice if already open', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		client.connect();
		expect(MockWebSocket.instances).toHaveLength(1);
	});

	it('schedules reconnect with backoff', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();
		MockWebSocket.instances[0].simulateClose();

		// First reconnect at 1s
		vi.advanceTimersByTime(999);
		expect(MockWebSocket.instances).toHaveLength(1);
		vi.advanceTimersByTime(1);
		expect(MockWebSocket.instances).toHaveLength(2);

		// Second reconnect at 2s
		MockWebSocket.instances[1].simulateClose();
		vi.advanceTimersByTime(1999);
		expect(MockWebSocket.instances).toHaveLength(2);
		vi.advanceTimersByTime(1);
		expect(MockWebSocket.instances).toHaveLength(3);
	});

	it('unsubscribe removes active subscription', async () => {
		const client = await loadClient();
		client.connect();
		MockWebSocket.instances[0].simulateOpen();

		client.subscribe('tasks', { view: 'today' });
		client.unsubscribe('tasks');

		// Disconnect and reconnect
		MockWebSocket.instances[0].simulateClose();
		vi.advanceTimersByTime(1000);
		MockWebSocket.instances[1].simulateOpen();

		// Should NOT resubscribe to tasks
		const sent = MockWebSocket.instances[1].sent;
		const subs = sent.filter((s) => JSON.parse(s).type === 'subscribe');
		expect(subs).toHaveLength(0);
	});
});
