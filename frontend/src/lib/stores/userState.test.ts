import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const patch = vi.fn();
vi.mock('../api/endpoints/state', () => ({
	state: { patch: () => patch() }
}));
vi.mock('../api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof import('../api/client')>();
	return { ...actual, getApiClient: () => ({}) };
});

import { userStateStore } from './userState.svelte';

/** A promise plus the handles to settle it, to hold a PATCH in flight. */
function deferred<T>() {
	let resolve!: (v: T) => void;
	let reject!: (e: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
}

describe('userStateStore.reconcileFromServer', () => {
	beforeEach(() => {
		patch.mockReset().mockResolvedValue({});
		userStateStore.clear();
	});
	afterEach(() => userStateStore.clear());

	it('applies server truth while no local write is outstanding', () => {
		userStateStore.reconcileFromServer({ activeContextId: 7 });
		expect(userStateStore.activeContextId).toBe(7);
	});

	it('treats a null server payload as an empty state', () => {
		userStateStore.setValue({ activeContextId: 7 });
		userStateStore.reconcileFromServer(null as never);
		expect(userStateStore.value).toEqual({});
	});

	// The mid-session /api/v1/config refresh races the user's own context switch.
	// The server copy is older than the optimistic value, and applying it would
	// visibly revert the switch AND retrigger a full page refetch through the
	// activeContextId effect on today/tomorrow/week.
	it('stands down while a context switch is still unacknowledged', async () => {
		const inflight = deferred<unknown>();
		patch.mockReturnValue(inflight.promise);

		const write = userStateStore.setActiveContextId(5);
		expect(userStateStore.activeContextId).toBe(5);

		userStateStore.reconcileFromServer({ activeContextId: 1 });
		expect(userStateStore.activeContextId).toBe(5);

		inflight.resolve({});
		await write;

		// Once acknowledged, the next refresh is authoritative again.
		userStateStore.reconcileFromServer({ activeContextId: 1 });
		expect(userStateStore.activeContextId).toBe(1);
	});

	// A rejected PATCH must not leave the counter stuck above zero — that would
	// freeze reconciliation for the rest of the session.
	it('resumes reconciling after a failed write', async () => {
		patch.mockRejectedValue(new Error('offline'));

		await expect(userStateStore.setActiveContextId(5)).rejects.toThrow();

		userStateStore.reconcileFromServer({ activeContextId: 1 });
		expect(userStateStore.activeContextId).toBe(1);
	});

	// Two rapid switches: the counter must not drop to zero on the first
	// acknowledgement while the second is still on the wire.
	it('stays paused until every outstanding write has settled', async () => {
		const first = deferred<unknown>();
		const second = deferred<unknown>();
		patch.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

		const a = userStateStore.setActiveContextId(5);
		const b = userStateStore.setActiveContextId(6);

		first.resolve({});
		await a;
		userStateStore.reconcileFromServer({ activeContextId: 1 });
		expect(userStateStore.activeContextId).toBe(6);

		second.resolve({});
		await b;
		userStateStore.reconcileFromServer({ activeContextId: 1 });
		expect(userStateStore.activeContextId).toBe(1);
	});
});
