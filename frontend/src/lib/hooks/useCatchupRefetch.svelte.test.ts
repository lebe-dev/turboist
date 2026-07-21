import { describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { useCatchupRefetch } from './useCatchupRefetch.svelte';

describe('useCatchupRefetch', () => {
	it('does not refetch while the signal is null', () => {
		const refetch = vi.fn();
		const stop = $effect.root(() => {
			useCatchupRefetch(() => null, refetch);
		});
		flushSync();
		expect(refetch).not.toHaveBeenCalled();
		stop();
	});

	it('refetches once when the signal advances to a fresh value', () => {
		let at = $state<number | null>(null);
		const refetch = vi.fn();
		const stop = $effect.root(() => {
			useCatchupRefetch(() => at, refetch);
		});
		flushSync();
		expect(refetch).not.toHaveBeenCalled();

		at = 1000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);
		stop();
	});

	it('dedupes the same value and refetches again only on a new one', () => {
		let at = $state<number | null>(null);
		const refetch = vi.fn();
		const stop = $effect.root(() => {
			useCatchupRefetch(() => at, refetch);
		});
		at = 1000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);

		// Re-flush without changing the signal → no additional refetch.
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);

		// A newer timestamp advances the signal → refetch again.
		at = 2000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(2);
		stop();
	});

	it('drives the same refetch from two independent signals (reconnect + sync)', () => {
		// Mirrors the (app) layout wiring: eventsClient.reconnectedAt and
		// statusStore.syncedAt each get their own subscription onto the shared
		// refreshAll(). Each latch is independent; a double refetch when both fire
		// is acceptable per §4.8.
		let reconnectedAt = $state<number | null>(null);
		let syncedAt = $state<number | null>(null);
		const refetch = vi.fn();
		const stop = $effect.root(() => {
			useCatchupRefetch(() => reconnectedAt, refetch);
			useCatchupRefetch(() => syncedAt, refetch);
		});
		flushSync();
		expect(refetch).not.toHaveBeenCalled();

		// SSE reconnect fires the catch-up.
		reconnectedAt = 1000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);

		// Replay completing (syncedAt bump) fires it again through its own latch,
		// even though it shares the reconnect timestamp value.
		syncedAt = 1000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(2);
		stop();
	});

	it('stops refetching after the effect root is disposed', () => {
		let at = $state<number | null>(null);
		const refetch = vi.fn();
		const stop = $effect.root(() => {
			useCatchupRefetch(() => at, refetch);
		});
		at = 1000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);

		stop();
		at = 2000;
		flushSync();
		expect(refetch).toHaveBeenCalledTimes(1);
	});
});
