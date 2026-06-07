import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import type { JoinPreview, JoinResult } from '$lib/api/types';
import { ApiError } from '$lib/api/errors';
import { PENDING_INVITE_STORAGE_KEY } from '$lib/federation/join';
import JoinPage from './+page.svelte';

const goto = vi.fn(async (_url: unknown) => {});
vi.mock('$app/navigation', () => ({
	goto: (url: unknown) => goto(url)
}));
vi.mock('$app/paths', () => ({
	resolve: (p: string) => p
}));

const preview = vi.fn<(...a: unknown[]) => Promise<JoinPreview>>();
const join = vi.fn<(...a: unknown[]) => Promise<JoinResult>>();
vi.mock('$lib/api/endpoints/federation', () => ({
	federation: {
		preview: (...args: unknown[]) => preview(args[1]),
		join: (...args: unknown[]) => join(args[1])
	}
}));

const projectsLoad = vi.fn(async () => []);
vi.mock('$lib/stores/projects.svelte', () => ({
	projectsStore: {
		load: () => projectsLoad()
	}
}));

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function setupAuth(authenticated: boolean) {
	const store = createAuthStore({ fetchImpl: vi.fn(async () => jsonResponse({})) as unknown as typeof fetch });
	if (authenticated) {
		store.user = { id: 1, username: 'eu', totpEnabled: false };
		store.accessToken = 'A';
		store.status = 'authenticated';
	} else {
		store.status = 'guest';
	}
	return store;
}

function setHash(hash: string) {
	Object.defineProperty(window, 'location', {
		configurable: true,
		value: { ...window.location, hash, origin: 'https://my-instance.tld' }
	});
}

const samplePreview: JoinPreview = {
	projectName: 'Roadmap',
	ownerInstanceUrl: 'https://alice.example',
	ownerDisplayName: 'alice.example',
	permissions: 'write',
	protocolVersion: 1
};

// A fragment opened on the JOINER instance (origin my-instance.tld): the owner
// (alice.example) is carried explicitly, so owner ≠ origin and the page runs the
// preview/join path rather than the owner-side "open in your instance" prompt.
const JOINER_HASH = '#invite=theid.thesecret&owner=https://alice.example';

beforeEach(() => {
	goto.mockClear();
	preview.mockReset();
	join.mockReset();
	projectsLoad.mockClear();
	sessionStorage.clear();
});

afterEach(() => {
	vi.restoreAllMocks();
	sessionStorage.clear();
});

describe('Join page (Federation v1 F2.1, US-2.1)', () => {
	it('parses the invite from the URL fragment (AC1)', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);

		render(JoinPage);

		await waitFor(() => expect(preview).toHaveBeenCalled());
		// The parsed id+secret is sent to the server-side preview, along with the
		// owner instance URL (carried in the fragment) so our instance knows where
		// to fetch — NOT the joiner's own origin.
		expect(preview).toHaveBeenCalledWith({
			inviteId: 'theid',
			secret: 'thesecret',
			ownerInstanceUrl: 'https://alice.example'
		});
	});

	it('shows the project preview server-side when authenticated (AC3)', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);

		render(JoinPage);

		expect(await screen.findByText('Roadmap')).toBeTruthy();
		// Owner identity is rendered as displayName @ instance.
		expect(screen.getByText(/alice\.example/)).toBeTruthy();
	});

	it('drives the accept stepper handshake → snapshot → done (AC4)', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);
		join.mockResolvedValue({ projectId: 42, projectName: 'Roadmap', permissions: 'write' });

		render(JoinPage);

		const acceptBtn = await screen.findByRole('button', { name: /accept/i });
		await fireEvent.click(acceptBtn);

		await waitFor(() =>
			expect(join).toHaveBeenCalledWith({
				inviteId: 'theid',
				secret: 'thesecret',
				ownerInstanceUrl: 'https://alice.example'
			})
		);
		// The final stepper state shows a success/done affordance.
		expect(await screen.findByText(/done|joined|success/i)).toBeTruthy();
	});

	it('stashes the invite and routes to login when unauthenticated, ready to resume (AC5)', async () => {
		setHash(JOINER_HASH);
		setupAuth(false);

		render(JoinPage);

		await waitFor(() => expect(goto).toHaveBeenCalled());
		// The unauthenticated visitor is sent to /login.
		const firstCall = goto.mock.calls[0] ?? [];
		expect(String(firstCall[0])).toContain('/login');
		// The invite + owner is stashed (in sessionStorage, never localStorage) so
		// the flow resumes after login.
		const stashed = sessionStorage.getItem(PENDING_INVITE_STORAGE_KEY);
		expect(stashed).toBeTruthy();
		if (typeof localStorage !== 'undefined') {
			expect(localStorage.getItem(PENDING_INVITE_STORAGE_KEY)).toBeNull();
		}
		expect(JSON.parse(stashed as string)).toEqual({
			inviteId: 'theid',
			secret: 'thesecret',
			owner: 'https://alice.example'
		});
		// Preview must NOT have been attempted while unauthenticated.
		expect(preview).not.toHaveBeenCalled();
	});

	it('resumes a stashed invite after login (AC5) — falls back to the session stash when the hash is gone', async () => {
		// No hash in the URL, but a pending join was stashed before login.
		setHash('');
		sessionStorage.setItem(
			PENDING_INVITE_STORAGE_KEY,
			JSON.stringify({ inviteId: 'theid', secret: 'thesecret', owner: 'https://alice.example' })
		);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);

		render(JoinPage);

		await waitFor(() =>
			expect(preview).toHaveBeenCalledWith({
				inviteId: 'theid',
				secret: 'thesecret',
				ownerInstanceUrl: 'https://alice.example'
			})
		);
		expect(await screen.findByText('Roadmap')).toBeTruthy();
	});

	it('on the owner instance, retargets to the visitor own instance carrying secret + owner in the fragment (AC2)', async () => {
		// Opened on the OWNER (no owner param → owner == page origin), so the page
		// shows the "open in your instance" prompt instead of trying to join here.
		let navigatedTo = '';
		Object.defineProperty(window, 'location', {
			configurable: true,
			value: {
				hash: '#invite=theid.thesecret',
				origin: 'https://my-instance.tld',
				set href(v: string) {
					navigatedTo = v;
				},
				get href() {
					return navigatedTo;
				}
			}
		});
		setupAuth(false);

		render(JoinPage);

		const input = await screen.findByPlaceholderText(/your-instance/i);
		await fireEvent.input(input, { target: { value: 'bob.example' } });
		await fireEvent.click(screen.getByRole('button', { name: /open in your instance/i }));

		// US-2.1 AC2 / R4: secret rides in the fragment, never the query, and the
		// owner (this origin) is carried so the joiner knows whom to handshake.
		expect(navigatedTo.startsWith('https://bob.example/federation/join#')).toBe(true);
		expect(navigatedTo).not.toContain('?');
		const fragment = navigatedTo.split('#')[1] ?? '';
		const fragParams = new URLSearchParams(fragment);
		expect(fragParams.get('invite')).toBe('theid.thesecret');
		expect(fragParams.get('owner')).toBe('https://my-instance.tld');
		// No join was attempted from the owner instance.
		expect(preview).not.toHaveBeenCalled();
	});

	it('surfaces an error when the invite hash is malformed', async () => {
		setHash('#nope');
		setupAuth(true);

		render(JoinPage);

		expect(await screen.findByText(/invalid|malformed|no invite/i)).toBeTruthy();
		expect(preview).not.toHaveBeenCalled();
	});
});

describe('Join page error mapping (Federation v1 F2.2, US-2.2 / US-9.1)', () => {
	async function acceptWithJoinError(code: string, status: number): Promise<void> {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);
		join.mockRejectedValue(new ApiError(code, 'owner says no', status));

		render(JoinPage);
		const acceptBtn = await screen.findByRole('button', { name: /accept/i });
		await fireEvent.click(acceptBtn);
		await waitFor(() => expect(join).toHaveBeenCalled());
	}

	it('maps a no-version-overlap 400 to the version-unsupported message (US-9.1 AC2)', async () => {
		await acceptWithJoinError('federation_version_unsupported', 400);
		expect(await screen.findByText(/protocol version your instance does not support/i)).toBeTruthy();
	});

	it('maps a generic 401 to the invite-invalid message without leaking id vs secret (US-2.2 AC4)', async () => {
		await acceptWithJoinError('federation_signature_invalid', 401);
		expect(await screen.findByText(/invalid, expired, revoked, or already used/i)).toBeTruthy();
	});

	it('maps a 409 to the key-mismatch message (US-2.2 AC5)', async () => {
		await acceptWithJoinError('federation_key_mismatch', 409);
		expect(await screen.findByText(/different key/i)).toBeTruthy();
	});

	it('maps an owner-untrusted 403 to the untrusted message (US-2.2 AC2)', async () => {
		await acceptWithJoinError('federation_untrusted', 403);
		expect(await screen.findByText(/identity could not be verified/i)).toBeTruthy();
	});

	it('maps a 410 Gone to the invite-gone message', async () => {
		await acceptWithJoinError('gone', 410);
		expect(await screen.findByText(/no longer valid/i)).toBeTruthy();
	});
});

describe('Join page snapshot bootstrap (Federation v1 F2.3, US-2.3)', () => {
	it('refreshes the projects store after a successful join so the new project appears (US-2.3)', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);
		join.mockResolvedValue({ projectId: 42, projectName: 'Roadmap', permissions: 'write' });

		render(JoinPage);

		const acceptBtn = await screen.findByRole('button', { name: /accept/i });
		await fireEvent.click(acceptBtn);

		await waitFor(() => expect(join).toHaveBeenCalled());
		// The federated project the snapshot bootstrap created is pulled into the
		// projects store so it shows up in the UI without a manual reload.
		await waitFor(() => expect(projectsLoad).toHaveBeenCalled());
		expect(await screen.findByText(/done|joined|success/i)).toBeTruthy();
	});

	it('surfaces a snapshot-stage failure and returns to the preview (US-2.3 AC5)', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);
		// A mid-stream snapshot failure on the backend rolls everything back and
		// surfaces an internal error; the page returns to the preview so the user
		// can retry rather than landing on a half-joined "done".
		join.mockRejectedValue(new ApiError('internal', 'snapshot stream failed', 500));

		render(JoinPage);
		const acceptBtn = await screen.findByRole('button', { name: /accept/i });
		await fireEvent.click(acceptBtn);

		await waitFor(() => expect(join).toHaveBeenCalled());
		// No "done" state — the join did not complete.
		expect(screen.queryByText(/you've joined|you have joined/i)).toBeNull();
		// The projects store is NOT reloaded on a failed bootstrap.
		expect(projectsLoad).not.toHaveBeenCalled();
		// An error is shown (the bootstrap-failed message or the raw error).
		expect(await screen.findByText(/snapshot|copy|could not join|failed/i)).toBeTruthy();
	});

	it('maps an expired snapshot token (401) to a re-handshake message', async () => {
		setHash(JOINER_HASH);
		setupAuth(true);
		preview.mockResolvedValue(samplePreview);
		// The owner's snapshot endpoint rejects an expired token as a generic 401;
		// it surfaces through the join transport as federation_signature_invalid.
		join.mockRejectedValue(new ApiError('federation_signature_invalid', 'snapshot token expired', 401));

		render(JoinPage);
		const acceptBtn = await screen.findByRole('button', { name: /accept/i });
		await fireEvent.click(acceptBtn);

		await waitFor(() => expect(join).toHaveBeenCalled());
		expect(projectsLoad).not.toHaveBeenCalled();
		expect(await screen.findByText(/invalid, expired, revoked, or already used/i)).toBeTruthy();
	});
});
