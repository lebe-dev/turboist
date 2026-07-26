import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import { appSettingsStore } from '$lib/stores/appSettings.svelte';
import { projectsStore } from '$lib/stores/projects.svelte';
import type { Project } from '$lib/api/types';
import ProjectSuggestionsSection from './ProjectSuggestionsSection.svelte';

interface CapturedRequest {
	url: string;
	method: string;
	body: unknown;
}

function project(id: number, title: string): Project {
	return {
		id,
		contextId: 1,
		title,
		description: '',
		color: '#fff',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory: null,
		createdAt: '',
		updatedAt: ''
	};
}

function makeFetchMock(captured: CapturedRequest[]): typeof fetch {
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

		if (url.endsWith('/api/v1/app-settings/project-suggestions') && method === 'PUT') {
			const requested = body as { projectSuggestions: unknown[] };
			return new Response(
				JSON.stringify({ autoLabels: [], projectSuggestions: requested.projectSuggestions }),
				{ status: 200, headers: { 'Content-Type': 'application/json' } }
			);
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

afterEach(() => {
	vi.restoreAllMocks();
	appSettingsStore.clear();
	projectsStore.clear();
});

describe('ProjectSuggestionsSection', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
		projectsStore.setItems([project(4, 'Infra'), project(7, 'Finance')]);
	});

	it('renders the empty state when no rules are configured', () => {
		appSettingsStore.setValue({ autoLabels: [], projectSuggestions: [] });
		setupAuth(makeFetchMock(captured));
		render(ProjectSuggestionsSection);

		expect(screen.getByText(/no project suggestion rules yet|правил пока нет/i)).toBeTruthy();
	});

	it('renders existing rules with their project titles', () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [{ mask: 'deploy', projectIds: [4, 7], ignoreCase: true }]
		});
		setupAuth(makeFetchMock(captured));
		render(ProjectSuggestionsSection);

		expect(screen.getAllByDisplayValue('deploy').length).toBeGreaterThan(0);
		expect(screen.getAllByText('Infra, Finance').length).toBeGreaterThan(0);
	});

	it('saves an edited mask trimmed via PUT', async () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [{ mask: 'deploy', projectIds: [4], ignoreCase: true }]
		});
		setupAuth(makeFetchMock(captured));
		render(ProjectSuggestionsSection);

		const [maskInput] = screen.getAllByDisplayValue('deploy');
		await fireEvent.input(maskInput, { target: { value: '  release  ' } });
		await fireEvent.click(screen.getAllByRole('button', { name: /^save$|^сохранить$/i })[0]);

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		const put = captured.find((r) => r.method === 'PUT')!;
		expect(put.body).toEqual({
			projectSuggestions: [{ mask: 'release', projectIds: [4], ignoreCase: true }]
		});
	});

	it('refuses to save a rule without projects', async () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [{ mask: 'deploy', projectIds: [] }] as never
		});
		setupAuth(makeFetchMock(captured));
		render(ProjectSuggestionsSection);

		const [maskInput] = screen.getAllByDisplayValue('deploy');
		await fireEvent.input(maskInput, { target: { value: 'release' } });
		await fireEvent.click(screen.getAllByRole('button', { name: /^save$|^сохранить$/i })[0]);

		expect(captured.some((r) => r.method === 'PUT')).toBe(false);
	});
});
