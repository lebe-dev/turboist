import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { toastMock, lockMock } = vi.hoisted(() => ({
	toastMock: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
	lockMock: vi.fn()
}));

vi.mock('svelte-sonner', () => ({ toast: toastMock }));
vi.mock('$lib/api/client', () => ({ getApiClient: () => ({}) }));
vi.mock('$lib/api/endpoints/today', () => ({ today: { lock: lockMock } }));

import { ApiError } from '$lib/api/errors';
import { todayLockStore } from '$lib/stores/todayLock.svelte';
import TodayLockButton from './TodayLockButton.svelte';

function future(hours: number): string {
	return new Date(Date.now() + hours * 3_600_000).toISOString();
}

function lockButton(): HTMLButtonElement {
	return screen.getByTestId('today-lock') as HTMLButtonElement;
}

beforeEach(() => {
	todayLockStore.clear();
});

afterEach(() => {
	lockMock.mockReset();
	toastMock.success.mockReset();
	toastMock.error.mockReset();
	toastMock.info.mockReset();
});

describe('TodayLockButton', () => {
	it('offers to lock the day when no lock runs', () => {
		render(TodayLockButton);
		const button = lockButton();
		expect(button.getAttribute('aria-label')).toBe('Lock today to its projects');
		expect(button.getAttribute('aria-pressed')).toBe('false');
	});

	it('explains itself in a tooltip on hover', async () => {
		// jsdom has no ResizeObserver, which the tooltip's positioning needs.
		vi.stubGlobal(
			'ResizeObserver',
			class {
				observe(): void {}
				unobserve(): void {}
				disconnect(): void {}
			}
		);
		render(TodayLockButton);
		await fireEvent.pointerMove(lockButton());
		await fireEvent.focus(lockButton());
		const tip = await screen.findAllByText(/Focus lock: until midnight/);
		expect(tip.length).toBeGreaterThan(0);
		vi.unstubAllGlobals();
	});

	it('asks for confirmation before locking and locks on confirm', async () => {
		lockMock.mockResolvedValue({ lockedUntil: future(5) });
		render(TodayLockButton);
		await fireEvent.click(lockButton());

		const dialog = await screen.findByRole('alertdialog');
		expect(dialog.textContent).toContain('Lock today to its projects?');
		expect(dialog.textContent).toContain('cannot be lifted before midnight');
		expect(lockMock).not.toHaveBeenCalled();

		await fireEvent.click(screen.getByRole('button', { name: 'Lock today' }));
		await waitFor(() => expect(lockMock).toHaveBeenCalledOnce());
		await waitFor(() => expect(toastMock.success).toHaveBeenCalledOnce());
		await waitFor(() => expect(lockButton().getAttribute('aria-pressed')).toBe('true'));
	});

	it('does nothing when the confirmation is cancelled', async () => {
		render(TodayLockButton);
		await fireEvent.click(lockButton());
		await screen.findByRole('alertdialog');
		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
		expect(lockMock).not.toHaveBeenCalled();
		expect(lockButton().getAttribute('aria-pressed')).toBe('false');
	});

	it('reports a failed lock and stays unlocked', async () => {
		lockMock.mockRejectedValue(new ApiError('network_error', 'Load failed', 0));
		render(TodayLockButton);
		await fireEvent.click(lockButton());
		await fireEvent.click(await screen.findByRole('button', { name: 'Lock today' }));
		await waitFor(() => expect(toastMock.error).toHaveBeenCalledOnce());
		expect(toastMock.success).not.toHaveBeenCalled();
		expect(lockButton().getAttribute('aria-pressed')).toBe('false');
	});

	it('shows a running lock and only explains it when tapped', async () => {
		todayLockStore.setUntil(future(3));
		render(TodayLockButton);
		const button = lockButton();
		expect(button.getAttribute('aria-pressed')).toBe('true');
		await fireEvent.click(button);
		expect(toastMock.info).toHaveBeenCalledWith(
			'Until midnight, today takes only today’s projects and this week’s tasks. Other days are unrestricted'
		);
		expect(screen.queryByRole('alertdialog')).toBeNull();
	});

	it('treats a lock that already ran out as off', () => {
		todayLockStore.setUntil(new Date(Date.now() - 60_000).toISOString());
		render(TodayLockButton);
		expect(lockButton().getAttribute('aria-pressed')).toBe('false');
	});
});
