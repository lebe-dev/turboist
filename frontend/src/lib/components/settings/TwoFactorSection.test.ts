import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import TwoFactorSection from './TwoFactorSection.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function emptyResponse(status = 204): Response {
	return new Response(null, { status });
}

function setupStore(totpEnabled: boolean, fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('TwoFactorSection', () => {
	it('shows enable button without a status badge when 2FA is off', () => {
		setupStore(false, vi.fn());
		render(TwoFactorSection);

		expect(screen.queryByTestId('twofa-status')).toBeNull();
		expect(screen.queryByTestId('twofa-unavailable')).toBeNull();
		expect(screen.getByRole('button', { name: /enable 2fa/i })).toBeTruthy();
	});

	it('disables the enable button and shows a notice when TOTP is not configured', () => {
		setupStore(false, vi.fn());
		render(TwoFactorSection, { props: { available: false } });

		expect(screen.getByTestId('twofa-unavailable')).toBeTruthy();
		const btn = screen.getByRole('button', { name: /enable 2fa/i }) as HTMLButtonElement;
		expect(btn.disabled).toBe(true);
	});

	it('shows enabled state with disable button when 2FA is on', () => {
		setupStore(true, vi.fn());
		render(TwoFactorSection);

		expect(screen.getByTestId('twofa-status').textContent).toMatch(/Enabled/i);
		expect(screen.getByRole('button', { name: /disable 2fa/i })).toBeTruthy();
	});

	it('walks through setup → confirm → recovery flow and marks user enabled', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({ secret: 'S3CRET', otpauthUrl: 'otpauth://...', qrPngBase64: 'AAAA' })
			)
			.mockResolvedValueOnce(jsonResponse({ recoveryCodes: ['AAAA1', 'BBBB2'] }));

		const store = setupStore(false, fetchMock as unknown as typeof fetch);
		render(TwoFactorSection);

		await fireEvent.click(screen.getByRole('button', { name: /enable 2fa/i }));

		// QR + secret shown
		const secretEl = await screen.findByTestId('twofa-secret');
		expect(secretEl.textContent).toBe('S3CRET');

		const codeInput = screen.getByPlaceholderText(/6-digit code/i) as HTMLInputElement;
		await fireEvent.input(codeInput, { target: { value: '123456' } });
		await fireEvent.click(screen.getByRole('button', { name: /^confirm$/i }));

		// Recovery codes shown, user flagged enabled
		const list = await screen.findByTestId('twofa-recovery-codes');
		expect(list.textContent).toContain('AAAA1');
		expect(list.textContent).toContain('BBBB2');
		expect(store.user?.totpEnabled).toBe(true);

		// Two API calls: setup then confirm
		expect(fetchMock).toHaveBeenCalledTimes(2);
		const confirmInit = fetchMock.mock.calls[1][1] as RequestInit;
		expect(confirmInit.method).toBe('POST');
		expect(confirmInit.body).toBe(JSON.stringify({ code: '123456' }));
	});

	it('disables 2FA after entering a valid code', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		fetchMock.mockResolvedValueOnce(emptyResponse(204));

		const store = setupStore(true, fetchMock as unknown as typeof fetch);
		render(TwoFactorSection);

		await fireEvent.click(screen.getByRole('button', { name: /disable 2fa/i }));

		const codeInput = screen.getByPlaceholderText(/totp or recovery/i) as HTMLInputElement;
		await fireEvent.input(codeInput, { target: { value: '654321' } });
		await fireEvent.click(screen.getByRole('button', { name: /^disable$/i }));

		await waitFor(() => expect(store.user?.totpEnabled).toBe(false));
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect(init.method).toBe('POST');
		expect(init.body).toBe(JSON.stringify({ code: '654321' }));
	});
});
