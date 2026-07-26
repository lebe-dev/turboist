import { afterEach, describe, expect, it, vi } from 'vitest';

const { toastMock } = vi.hoisted(() => ({
	toastMock: { success: vi.fn(), error: vi.fn() }
}));

vi.mock('svelte-sonner', () => ({
	toast: toastMock
}));

import { usePageLoad } from './usePageLoad.svelte';

describe('usePageLoad', () => {
	afterEach(() => {
		toastMock.success.mockReset();
		toastMock.error.mockReset();
	});

	it('does not fetch when autoLoad is false', () => {
		const fetcher = vi.fn(async () => undefined);
		const page = usePageLoad(fetcher, { autoLoad: false });
		expect(fetcher).not.toHaveBeenCalled();
		expect(page.loading).toBe(false);
		expect(page.error).toBeNull();
	});

	it('runs the fetcher on refetch and clears loading', async () => {
		const fetcher = vi.fn(async () => undefined);
		const page = usePageLoad(fetcher, { autoLoad: false });
		await page.refetch();
		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(page.loading).toBe(false);
		expect(page.error).toBeNull();
	});

	it('records error and shows toast on failure', async () => {
		const fetcher = vi.fn(async () => {
			throw new Error('nope');
		});
		const page = usePageLoad(fetcher, { autoLoad: false, errorMessage: 'Load failed' });
		await page.refetch();
		expect(page.loading).toBe(false);
		expect(page.error).toBe('nope');
		expect(toastMock.error).toHaveBeenCalledWith('nope');
	});

	it('uses the configured fallback when the error has no message', async () => {
		const fetcher = vi.fn(async () => {
			throw 'string-throw';
		});
		const page = usePageLoad(fetcher, { autoLoad: false, errorMessage: 'Load failed' });
		await page.refetch();
		expect(page.error).toBe('Load failed');
		expect(toastMock.error).toHaveBeenCalledWith('Load failed');
	});

	it('routes errors through onError instead of the toast when provided', async () => {
		const onError = vi.fn();
		const err = new Error('custom');
		const fetcher = vi.fn(async () => {
			throw err;
		});
		const page = usePageLoad(fetcher, { autoLoad: false, onError });
		await page.refetch();
		expect(onError).toHaveBeenCalledWith(err);
		expect(toastMock.error).not.toHaveBeenCalled();
		expect(page.error).toBe('custom');
	});

	it('cancels in-flight requests so their resolution is ignored', async () => {
		let release: () => void;
		const blocking = new Promise<void>((resolve) => {
			release = resolve;
		});
		const fetcher = vi.fn(async (isValid: () => boolean) => {
			await blocking;
			// fetcher resolves after cancel — usePageLoad must ignore this completion.
			expect(isValid()).toBe(false);
		});
		const page = usePageLoad(fetcher, { autoLoad: false });
		const pending = page.refetch();
		expect(page.loading).toBe(true);
		page.cancel();
		expect(page.loading).toBe(false);
		release!();
		await pending;
		expect(page.loading).toBe(false);
		expect(page.error).toBeNull();
	});

	it('keeps the most recent error when refetch is called twice', async () => {
		const errors = [new Error('first'), new Error('second')];
		let i = 0;
		const fetcher = vi.fn(async () => {
			throw errors[i++];
		});
		const page = usePageLoad(fetcher, { autoLoad: false });
		await page.refetch();
		await page.refetch();
		expect(page.error).toBe('second');
	});

	it('revalidate() called during refetch does not prevent loading from resetting', async () => {
		let releaseRefetch: () => void;
		const blockingRefetch = new Promise<void>((resolve) => {
			releaseRefetch = resolve;
		});
		let call = 0;
		const fetcher = vi.fn(async () => {
			if (call++ === 0) {
				// First call is the refetch — block it so revalidate can race
				await blockingRefetch;
			}
		});
		const page = usePageLoad(fetcher, { autoLoad: false });
		const refetchDone = page.refetch();
		expect(page.loading).toBe(true);
		// Simulate SSE-driven revalidate arriving while refetch is still in flight
		const revalidateDone = page.revalidate();
		// Release the blocked refetch
		releaseRefetch!();
		await refetchDone;
		await revalidateDone;
		// loading must be false — revalidate() must not have prevented the reset
		expect(page.loading).toBe(false);
	});

	describe('epoch guard', () => {
		it('invalidates a revalidation whose response lands after a local mutation', async () => {
			// The tab-wake bug: an SSE reconnect fires a background revalidate, the user
			// completes a task while it is in flight, and the pre-mutation snapshot then
			// overwrites the optimistic removal so the task visibly reappears.
			let epoch = 0;
			let release: () => void;
			const blocking = new Promise<void>((resolve) => {
				release = resolve;
			});
			const seen: boolean[] = [];
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				await blocking;
				seen.push(isValid());
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });

			const pending = page.revalidate();
			epoch += 1; // the user completes a task
			release!();
			await pending;

			// The stale snapshot is refused. (The retry that follows is covered below.)
			expect(seen[0]).toBe(false);
		});

		it('keeps a revalidation valid when no mutation happened', async () => {
			const seen: boolean[] = [];
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				seen.push(isValid());
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => 7 });
			await page.revalidate();
			expect(seen).toEqual([true]);
		});

		it('does NOT invalidate a foreground refetch on a concurrent mutation', async () => {
			// refetch() is user intent (context switch, route change): its result must
			// land even if a mutation raced it, or the view keeps the previous filter.
			let epoch = 0;
			let release: () => void;
			const blocking = new Promise<void>((resolve) => {
				release = resolve;
			});
			const seen: boolean[] = [];
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				await blocking;
				seen.push(isValid());
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });

			const pending = page.refetch();
			epoch += 1;
			release!();
			await pending;

			expect(seen).toEqual([true]);
			expect(page.loading).toBe(false);
		});

		it('retries once after discarding a snapshot, so a remote change is not lost', async () => {
			let epoch = 0;
			const calls: Array<{ valid: boolean }> = [];
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				if (calls.length === 0) epoch += 1; // the user mutates mid-flight
				calls.push({ valid: isValid() });
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });

			await page.revalidate();

			// First attempt discarded, second one applied.
			expect(calls).toEqual([{ valid: false }, { valid: true }]);
		});

		it('does not retry more than once when the user keeps mutating', async () => {
			let epoch = 0;
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				epoch += 1; // every attempt races another local write
				isValid();
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });

			await page.revalidate();

			expect(fetcher).toHaveBeenCalledTimes(2);
		});

		it('does not retry when the fetch was superseded by a newer load', async () => {
			// A newer refetch/revalidate already owns the view; retrying would be a
			// pointless third request.
			let release: () => void;
			const blocking = new Promise<void>((resolve) => {
				release = resolve;
			});
			let call = 0;
			const fetcher = vi.fn(async () => {
				if (call++ === 0) await blocking;
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => 0 });

			const first = page.revalidate();
			const second = page.revalidate();
			release!();
			await Promise.all([first, second]);

			expect(fetcher).toHaveBeenCalledTimes(2);
		});

		it('does not retry after a failed revalidation', async () => {
			let epoch = 0;
			const fetcher = vi.fn(async () => {
				epoch += 1;
				throw new Error('boom');
			});
			const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });

			await page.revalidate();

			expect(fetcher).toHaveBeenCalledTimes(1);
			warn.mockRestore();
		});

		it('re-arms the guard on each revalidation', async () => {
			let epoch = 0;
			const seen: boolean[] = [];
			const fetcher = vi.fn(async (isValid: () => boolean) => {
				seen.push(isValid());
			});
			const page = usePageLoad(fetcher, { autoLoad: false, epoch: () => epoch });
			epoch += 1;
			await page.revalidate();
			epoch += 1;
			await page.revalidate();
			// A mutation before the fetch starts is not a reason to drop it.
			expect(seen).toEqual([true, true]);
		});
	});
});
