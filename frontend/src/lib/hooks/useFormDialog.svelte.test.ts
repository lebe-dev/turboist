import { afterEach, describe, expect, it, vi } from 'vitest';

const { toastMock } = vi.hoisted(() => ({
	toastMock: { success: vi.fn(), error: vi.fn() }
}));

vi.mock('svelte-sonner', () => ({
	toast: toastMock
}));

import { ApiError } from '$lib/api/errors';
import { useFormDialog } from './useFormDialog.svelte';

describe('useFormDialog', () => {
	afterEach(() => {
		toastMock.success.mockReset();
		toastMock.error.mockReset();
	});

	it('starts with submitting=false', () => {
		const dlg = useFormDialog();
		expect(dlg.submitting).toBe(false);
	});

	it('toggles submitting around a successful submit and shows success toast', async () => {
		const dlg = useFormDialog();
		let observed: boolean | null = null;
		const promise = dlg.submit(
			async () => {
				observed = dlg.submitting;
				return 42;
			},
			{ success: 'Saved', error: 'Failed' }
		);
		expect(dlg.submitting).toBe(true);
		const result = await promise;
		expect(observed).toBe(true);
		expect(result).toBe(42);
		expect(dlg.submitting).toBe(false);
		expect(toastMock.success).toHaveBeenCalledWith('Saved');
		expect(toastMock.error).not.toHaveBeenCalled();
	});

	it('shows error toast and resets submitting on failure', async () => {
		const dlg = useFormDialog();
		const result = await dlg.submit(
			async () => {
				throw new Error('boom');
			},
			{ success: 'Saved', error: 'Failed' }
		);
		expect(result).toBeUndefined();
		expect(dlg.submitting).toBe(false);
		// describeError prefers the underlying Error message over the fallback
		expect(toastMock.error).toHaveBeenCalledWith('boom');
		expect(toastMock.success).not.toHaveBeenCalled();
	});

	it('uses the fallback error message when ApiError has no message', async () => {
		const dlg = useFormDialog();
		await dlg.submit(
			async () => {
				throw new ApiError('internal_error', '', 500);
			},
			{ success: 'Saved', error: 'Failed' }
		);
		expect(toastMock.error).toHaveBeenCalledWith('Failed');
	});

	it('ignores re-entrant calls while a submit is in flight', async () => {
		const dlg = useFormDialog();
		let release: (v: number) => void;
		const blocking = new Promise<number>((resolve) => {
			release = resolve;
		});
		const fnA = vi.fn(async () => blocking);
		const fnB = vi.fn(async () => 2);

		const pa = dlg.submit(fnA, { success: 'A', error: 'AE' });
		const pb = dlg.submit(fnB, { success: 'B', error: 'BE' });
		expect(await pb).toBeUndefined();
		expect(fnB).not.toHaveBeenCalled();

		release!(1);
		expect(await pa).toBe(1);
		expect(fnA).toHaveBeenCalledTimes(1);
	});
});
