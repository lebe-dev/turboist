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

	it('refetch resolves to true on success and false on failure', async () => {
		// The success flag lets a user-initiated caller (e.g. the F3.4 explicit
		// Reload) react to a failure, unlike the error-swallowing revalidate() path.
		const ok = usePageLoad(vi.fn(async () => undefined), { autoLoad: false });
		expect(await ok.refetch()).toBe(true);

		const bad = usePageLoad(
			vi.fn(async () => {
				throw new Error('boom');
			}),
			{ autoLoad: false, onError: vi.fn() }
		);
		expect(await bad.refetch()).toBe(false);
	});

	it('refetch resolves to false when it was superseded by a later refetch', async () => {
		// A cancelled/superseded refetch must not report success — a Reload racing a
		// route change should not clear its banner on a stale resolution.
		let release: () => void;
		const blocking = new Promise<void>((resolve) => {
			release = resolve;
		});
		let call = 0;
		const fetcher = vi.fn(async () => {
			if (call++ === 0) await blocking;
		});
		const page = usePageLoad(fetcher, { autoLoad: false });
		const first = page.refetch();
		const second = page.refetch();
		release!();
		expect(await first).toBe(false);
		expect(await second).toBe(true);
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
		const fetcher = vi.fn(async (isValid: () => boolean) => {
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
});
