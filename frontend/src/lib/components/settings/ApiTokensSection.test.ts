import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import ApiTokensSection from './ApiTokensSection.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function emptyTokensList(): Response {
	return jsonResponse([]);
}

interface CapturedRequest {
	url: string;
	method: string;
	body: unknown;
}

function makeFetchMock(tokensListResponse: Response, captured: CapturedRequest[]): typeof fetch {
	return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === 'string' ? input : input.toString();
		const method = (init?.method ?? 'GET').toUpperCase();
		let body: unknown = undefined;
		if (typeof init?.body === 'string') {
			try {
				body = JSON.parse(init.body);
			} catch {
				body = init.body;
			}
		}
		captured.push({ url, method, body });

		if (url.endsWith('/api/v1/api-tokens') && method === 'GET') {
			return tokensListResponse.clone();
		}
		if (url.endsWith('/api/v1/api-tokens') && method === 'POST') {
			const requested = body as { name: string; scopes: string[] };
			return jsonResponse({
				id: 42,
				name: requested.name,
				scopes: requested.scopes,
				createdAt: '2026-01-01T00:00:00.000Z',
				token: 'tok_secret_value'
			});
		}
		return new Response(null, { status: 404 });
	}) as unknown as typeof fetch;
}

function setupAuth(fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled: false };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

function getReadCheckbox(resource: string): HTMLElement {
	return screen.getByRole('checkbox', { name: new RegExp(`${resource}.*read`, 'i') });
}

function getWriteCheckbox(resource: string): HTMLElement {
	return screen.getByRole('checkbox', { name: new RegExp(`${resource}.*write`, 'i') });
}

function isChecked(el: HTMLElement): boolean {
	return el.getAttribute('aria-checked') === 'true' || el.dataset.state === 'checked';
}

function isDisabled(el: HTMLElement): boolean {
	return (
		el.hasAttribute('disabled') ||
		el.getAttribute('aria-disabled') === 'true' ||
		el.dataset.disabled === 'true' ||
		el.dataset.disabled === ''
	);
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('ApiTokensSection — scopes UI', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
	});

	async function renderEmpty() {
		const fetchMock = makeFetchMock(emptyTokensList(), captured);
		setupAuth(fetchMock);
		render(ApiTokensSection);
		// wait for initial GET
		await waitFor(() => expect(captured.some((r) => r.method === 'GET')).toBe(true));
	}

	it('"Read-only" preset checks all read checkboxes and leaves writes unchecked', async () => {
		await renderEmpty();

		await fireEvent.click(screen.getByRole('button', { name: /read-only|только чтение/i }));

		const resources = ['Tasks', 'Projects', 'Contexts', 'Labels', 'Sections', 'Troiki', 'Settings', 'Search', 'Calendars'];
		for (const res of resources) {
			expect(isChecked(getReadCheckbox(res))).toBe(true);
		}
		// writes off
		for (const res of ['Tasks', 'Projects', 'Contexts', 'Labels', 'Sections', 'Troiki', 'Settings']) {
			expect(isChecked(getWriteCheckbox(res))).toBe(false);
		}
	});

	it('"Full access" preset submits scopes ["*"]', async () => {
		await renderEmpty();

		await fireEvent.input(screen.getByPlaceholderText(/token name|имя токена/i), {
			target: { value: 'my-token' }
		});
		await fireEvent.click(screen.getByRole('button', { name: /^full access$|^полный доступ$/i }));
		await fireEvent.click(screen.getByRole('button', { name: /generate token|создать токен/i }));

		await waitFor(() => expect(captured.some((r) => r.method === 'POST')).toBe(true));
		const post = captured.find((r) => r.method === 'POST')!;
		expect(post.body).toEqual({ name: 'my-token', scopes: ['*'] });
	});

	it('"Tasks (read + write)" preset sets only tasks:read and tasks:write', async () => {
		await renderEmpty();

		await fireEvent.input(screen.getByPlaceholderText(/token name|имя токена/i), {
			target: { value: 'tasks-token' }
		});
		await fireEvent.click(screen.getByRole('button', { name: /tasks \(read \+ write\)|задачи/i }));
		await fireEvent.click(screen.getByRole('button', { name: /generate token|создать токен/i }));

		await waitFor(() => expect(captured.some((r) => r.method === 'POST')).toBe(true));
		const post = captured.find((r) => r.method === 'POST')!;
		expect(post.body).toEqual({
			name: 'tasks-token',
			scopes: ['tasks:read', 'tasks:write']
		});
	});

	it('toggling write auto-checks read and disables it; un-checking read clears write', async () => {
		await renderEmpty();

		const projectsRead = getReadCheckbox('Projects');
		const projectsWrite = getWriteCheckbox('Projects');

		expect(isChecked(projectsRead)).toBe(false);
		expect(isChecked(projectsWrite)).toBe(false);

		await fireEvent.click(projectsWrite);

		expect(isChecked(projectsWrite)).toBe(true);
		expect(isChecked(projectsRead)).toBe(true);
		expect(isDisabled(projectsRead)).toBe(true);

		// Now turn off write; read should remain on but become enabled
		await fireEvent.click(projectsWrite);
		expect(isChecked(projectsWrite)).toBe(false);
		expect(isChecked(projectsRead)).toBe(true);
		expect(isDisabled(projectsRead)).toBe(false);

		// Toggle write on again then un-check read => write should also be off
		await fireEvent.click(projectsWrite);
		expect(isChecked(projectsWrite)).toBe(true);
		// read is disabled while write is on, so first turn write off again to manipulate read
		await fireEvent.click(projectsWrite);
		expect(isDisabled(projectsRead)).toBe(false);
		await fireEvent.click(projectsRead);
		expect(isChecked(projectsRead)).toBe(false);
		expect(isChecked(projectsWrite)).toBe(false);
	});

	it('submits normalized scopes (write implies read, deduplicated)', async () => {
		await renderEmpty();

		await fireEvent.input(screen.getByPlaceholderText(/token name|имя токена/i), {
			target: { value: 'mixed' }
		});

		// Manually select: projects write (=> read auto), labels read, contexts write
		await fireEvent.click(getWriteCheckbox('Projects'));
		await fireEvent.click(getReadCheckbox('Labels'));
		await fireEvent.click(getWriteCheckbox('Contexts'));

		await fireEvent.click(screen.getByRole('button', { name: /generate token|создать токен/i }));

		await waitFor(() => expect(captured.some((r) => r.method === 'POST')).toBe(true));
		const post = captured.find((r) => r.method === 'POST')!;
		const body = post.body as { name: string; scopes: string[] };
		expect(body.name).toBe('mixed');

		// Sorted comparison — order in the array doesn't matter as long as required scopes present
		const got = [...body.scopes].sort();
		expect(got).toEqual(
			['projects:read', 'projects:write', 'labels:read', 'contexts:read', 'contexts:write'].sort()
		);

		// No duplicates
		expect(new Set(got).size).toBe(got.length);
	});
});

describe('ApiTokensSection — token list badges', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('shows "Full access" badge for tokens with ["*"]', async () => {
		const captured: CapturedRequest[] = [];
		const tokens = [
			{
				id: 1,
				name: 'full-token',
				scopes: ['*'],
				createdAt: '2026-01-01T00:00:00.000Z'
			}
		];
		const fetchMock = makeFetchMock(jsonResponse(tokens), captured);
		setupAuth(fetchMock);
		render(ApiTokensSection);

		const item = await screen.findByText('full-token');
		const li = item.closest('li')!;
		const badge = within(li).getByText(/full access|полный доступ/i);
		expect(badge).toBeTruthy();
	});

	it('shows individual scope badges for tokens with specific scopes', async () => {
		const captured: CapturedRequest[] = [];
		const tokens = [
			{
				id: 2,
				name: 'limited',
				scopes: ['tasks:read', 'tasks:write', 'projects:read'],
				createdAt: '2026-01-01T00:00:00.000Z'
			}
		];
		const fetchMock = makeFetchMock(jsonResponse(tokens), captured);
		setupAuth(fetchMock);
		render(ApiTokensSection);

		const item = await screen.findByText('limited');
		const li = item.closest('li')!;
		// "Full access" badge must NOT appear
		expect(within(li).queryByText(/^full access$|^полный доступ$/i)).toBeNull();

		// Each scope rendered as a badge using the human-readable format "Resource: action"
		expect(within(li).getByText(/tasks.*read|задачи.*чтение/i)).toBeTruthy();
		expect(within(li).getByText(/tasks.*write|задачи.*запись/i)).toBeTruthy();
		expect(within(li).getByText(/projects.*read|проекты.*чтение/i)).toBeTruthy();
	});
});
