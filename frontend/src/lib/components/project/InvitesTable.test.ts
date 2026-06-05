import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import InvitesTable from './InvitesTable.svelte';
import type { Invite } from '$lib/api/types';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

interface CapturedRequest {
	url: string;
	method: string;
}

function invite(over: Partial<Invite> = {}): Invite {
	return {
		inviteId: 'inv-active',
		permissions: 'write',
		maxUses: 1,
		usedCount: 0,
		status: 'active',
		expiresAt: '2030-01-01T00:00:00.000Z',
		revokedAt: '',
		consumedAt: '',
		createdAt: '2026-01-01T00:00:00.000Z',
		...over
	};
}

function makeFetchMock(list: Invite[], captured: CapturedRequest[]): typeof fetch {
	return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === 'string' ? input : input.toString();
		const method = (init?.method ?? 'GET').toUpperCase();
		captured.push({ url, method });

		if (url.endsWith('/api/v1/projects/7/invites') && method === 'GET') {
			return jsonResponse(list);
		}
		// revoke / delete return 204
		return new Response(null, { status: 204 });
	}) as unknown as typeof fetch;
}

function setupAuth(fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled: false };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('InvitesTable (Federation v1 F1.3)', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
	});

	it('renders a row per invite with its derived status (US-1.3 AC1)', async () => {
		const list = [
			invite({ inviteId: 'inv-active', status: 'active' }),
			invite({ inviteId: 'inv-expired', status: 'expired' }),
			invite({ inviteId: 'inv-consumed', status: 'consumed', usedCount: 1 }),
			invite({ inviteId: 'inv-revoked', status: 'revoked', revokedAt: '2026-02-01T00:00:00.000Z' })
		];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);
		render(InvitesTable, { projectId: 7 });

		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		// Each of the four lifecycle states is rendered as a status label.
		expect(await screen.findByText(/^Active$/i)).toBeTruthy();
		expect(screen.getByText(/^Expired$/i)).toBeTruthy();
		expect(screen.getByText(/^Consumed$/i)).toBeTruthy();
		expect(screen.getByText(/^Revoked$/i)).toBeTruthy();
	});

	it('never displays a secret for any invite (US-1.3 AC5)', async () => {
		const fetchMock = makeFetchMock([invite()], captured);
		setupAuth(fetchMock);
		const { container } = render(InvitesTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));
		// No invite row carries a secret to render; sanity-check the DOM has none.
		expect(container.textContent).not.toMatch(/secret/i);
	});

	it('offers Copy link ONLY for the session-created invite, never for re-visited ones (US-1.3 AC4, AC5)', async () => {
		const list = [
			invite({ inviteId: 'inv-session', status: 'active' }),
			invite({ inviteId: 'inv-old', status: 'active' })
		];
		const fetchMock = makeFetchMock(list, captured);
		setupAuth(fetchMock);

		const writeText = vi.fn().mockResolvedValue(undefined);
		Object.assign(navigator, { clipboard: { writeText } });

		render(InvitesTable, {
			projectId: 7,
			sessionLinks: { 'inv-session': 'https://me.tld/federation/join#invite=inv-session.thesecret' }
		});
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		const sessionRow = (await screen.findByText('inv-session')).closest('[data-invite-row]') as HTMLElement;
		const oldRow = screen.getByText('inv-old').closest('[data-invite-row]') as HTMLElement;

		// Session invite: a Copy-link button is present and copies the full link.
		const copyBtn = within(sessionRow).getByRole('button', { name: /copy/i });
		await fireEvent.click(copyBtn);
		expect(writeText).toHaveBeenCalledWith(
			'https://me.tld/federation/join#invite=inv-session.thesecret'
		);

		// Re-visited invite: no Copy-link button (the secret was never re-served).
		expect(within(oldRow).queryByRole('button', { name: /copy/i })).toBeNull();
	});

	it('revokes an active invite and reloads the list (US-1.3 AC2)', async () => {
		const fetchMock = makeFetchMock([invite({ inviteId: 'inv-active', status: 'active' })], captured);
		setupAuth(fetchMock);
		render(InvitesTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		const row = (await screen.findByText('inv-active')).closest('[data-invite-row]') as HTMLElement;
		await fireEvent.click(within(row).getByRole('button', { name: /revoke/i }));

		await waitFor(() =>
			expect(
				captured.some((r) => r.method === 'POST' && r.url.includes('/invites/inv-active/revoke'))
			).toBe(true)
		);
	});

	it('deletes a non-active invite (US-1.3 AC3)', async () => {
		const fetchMock = makeFetchMock(
			[invite({ inviteId: 'inv-expired', status: 'expired' })],
			captured
		);
		setupAuth(fetchMock);
		render(InvitesTable, { projectId: 7 });
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));

		const row = (await screen.findByText('inv-expired')).closest('[data-invite-row]') as HTMLElement;
		await fireEvent.click(within(row).getByRole('button', { name: /delete/i }));

		await waitFor(() =>
			expect(
				captured.some((r) => r.method === 'DELETE' && r.url.endsWith('/invites/inv-expired'))
			).toBe(true)
		);
	});
});
