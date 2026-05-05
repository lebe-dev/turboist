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
});
