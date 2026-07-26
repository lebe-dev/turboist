import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { issueTicket } = vi.hoisted(() => ({
	issueTicket: vi.fn(async () => ({ ticket: 'tkt' }))
}));

vi.mock('../api/endpoints/events', () => ({
	events: { issueTicket }
}));

vi.mock('../api/client', () => ({
	getApiClient: () => ({ baseUrl: '' })
}));

/** Minimal EventSource stand-in: records instances and lets tests drive readyState. */
class FakeEventSource {
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSED = 2;
	static instances: FakeEventSource[] = [];

	readyState = FakeEventSource.CONNECTING;
	closed = false;
	onopen: (() => void) | null = null;
	onerror: (() => void) | null = null;
	readonly listeners = new Map<string, Set<() => void>>();

	constructor(readonly url: string) {
		FakeEventSource.instances.push(this);
	}

	addEventListener(type: string, fn: () => void): void {
		let set = this.listeners.get(type);
		if (!set) {
			set = new Set();
			this.listeners.set(type, set);
		}
		set.add(fn);
	}

	close(): void {
		this.closed = true;
		this.readyState = FakeEventSource.CLOSED;
	}

	/** Simulate the server accepting the stream. */
	open(): void {
		this.readyState = FakeEventSource.OPEN;
		this.onopen?.();
	}

	/** Simulate a drop the browser gave up on (readyState CLOSED). */
	fail(): void {
		this.readyState = FakeEventSource.CLOSED;
		this.onerror?.();
	}

	/** Simulate the drop a suspended tab wakes into: native retry, still pending. */
	stall(): void {
		this.readyState = FakeEventSource.CONNECTING;
		this.onerror?.();
	}

	emit(type: string): void {
		for (const fn of this.listeners.get(type) ?? []) fn();
	}
}

vi.stubGlobal('EventSource', FakeEventSource);

/** Lets the pending issueTicket promise settle so connect() installs the source. */
const settle = () => new Promise<void>((r) => setTimeout(r, 0));

let eventsClient: typeof import('./events.svelte').eventsClient;

describe('eventsClient', () => {
	beforeEach(async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		FakeEventSource.instances = [];
		issueTicket.mockClear();
		// Fresh module per test: the client is a singleton with connection state.
		vi.resetModules();
		({ eventsClient } = await import('./events.svelte'));
	});

	afterEach(() => {
		eventsClient.stop();
		vi.useRealTimers();
	});

	it('opens a stream on start and dispatches scoped events to handlers', async () => {
		const onTasks = vi.fn();
		eventsClient.on('tasks', onTasks);
		eventsClient.start();
		await settle();

		const es = FakeEventSource.instances[0];
		expect(es.url).toBe('/api/v1/events?ticket=tkt');
		es.open();
		expect(eventsClient.connected).toBe(true);

		es.emit('tasks');
		expect(onTasks).toHaveBeenCalledWith('tasks');
	});

	it('reports reconnectedAt only after a real drop', async () => {
		eventsClient.start();
		await settle();
		FakeEventSource.instances[0].open();
		// First connect is not a reconnect — nothing was missed.
		expect(eventsClient.reconnectedAt).toBeNull();

		FakeEventSource.instances[0].fail();
		expect(eventsClient.connected).toBe(false);

		await vi.advanceTimersByTimeAsync(1_000);
		await settle();
		FakeEventSource.instances[1].open();
		expect(eventsClient.reconnectedAt).not.toBeNull();
	});

	describe('resume', () => {
		// The reported UX bug: a hidden tab's throttled reconnect timer fires seconds
		// into the user's first interaction, and its catch-up refresh lands mid-click.
		it('reconnects immediately instead of waiting out the backoff', async () => {
			eventsClient.start();
			await settle();
			FakeEventSource.instances[0].open();
			FakeEventSource.instances[0].fail();
			expect(FakeEventSource.instances).toHaveLength(1);

			eventsClient.resume();
			await settle();

			expect(FakeEventSource.instances).toHaveLength(2);
			// …and the pending backoff attempt must not open a third stream.
			await vi.advanceTimersByTimeAsync(60_000);
			await settle();
			expect(FakeEventSource.instances).toHaveLength(2);
		});

		it('tears down a source still stuck in CONNECTING so the ticket is re-issued', async () => {
			eventsClient.start();
			await settle();
			const first = FakeEventSource.instances[0];
			first.open();
			first.stall(); // native retry in progress: readyState CONNECTING, not CLOSED

			eventsClient.resume();
			await settle();

			expect(first.closed).toBe(true);
			expect(FakeEventSource.instances).toHaveLength(2);
			expect(issueTicket).toHaveBeenCalledTimes(2);
		});

		it('leaves a healthy open stream alone', async () => {
			eventsClient.start();
			await settle();
			FakeEventSource.instances[0].open();

			eventsClient.resume();
			await settle();

			expect(FakeEventSource.instances).toHaveLength(1);
			expect(FakeEventSource.instances[0].closed).toBe(false);
		});

		it('does nothing before start()', async () => {
			eventsClient.resume();
			await settle();
			expect(FakeEventSource.instances).toHaveLength(0);
			expect(issueTicket).not.toHaveBeenCalled();
		});

		it('resets the backoff so a later drop retries at the shortest delay', async () => {
			eventsClient.start();
			await settle();
			FakeEventSource.instances[0].open();

			// Three failed handshakes push the backoff out to 8s.
			for (let i = 0; i < 3; i++) {
				FakeEventSource.instances.at(-1)!.fail();
				await vi.advanceTimersByTimeAsync(10_000);
				await settle();
			}
			const beforeResume = FakeEventSource.instances.length;

			eventsClient.resume();
			await settle();
			expect(FakeEventSource.instances).toHaveLength(beforeResume + 1);
			FakeEventSource.instances.at(-1)!.open();
			FakeEventSource.instances.at(-1)!.fail();

			// attempt was reset, so the next retry is the 1s step, not 8s.
			await vi.advanceTimersByTimeAsync(1_000);
			await settle();
			expect(FakeEventSource.instances).toHaveLength(beforeResume + 2);
		});
	});
});
