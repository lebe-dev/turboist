import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import { appSettingsStore } from '$lib/stores/appSettings.svelte';
import type { AppSettings, LinkTitlesSettings } from '$lib/api/types';
import LinkTitlesSection from './LinkTitlesSection.svelte';

interface CapturedRequest {
	url: string;
	method: string;
	body: unknown;
}

function settings(linkTitles: LinkTitlesSettings): AppSettings {
	return {
		autoLabels: [],
		projectSuggestions: [],
		inboxProcessing: { prompt: '', paused: false },
		linkTitles
	};
}

// Answers PUT /link-titles the way the server does: merges the patch and
// normalises rule domains.
function makeFetchMock(captured: CapturedRequest[], current: LinkTitlesSettings): typeof fetch {
	return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === 'string' ? input : input.toString();
		const method = (init?.method ?? 'GET').toUpperCase();
		const body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined;
		captured.push({ url, method, body });
		if (url.endsWith('/api/v1/app-settings/link-titles') && method === 'PUT') {
			const patch = body as Partial<LinkTitlesSettings>;
			const next: LinkTitlesSettings = {
				...current,
				...patch,
				rules: (patch.rules ?? current.rules).map((r) => ({
					...r,
					domain: r.domain
						.replace(/^https?:\/\//, '')
						.replace(/^www\./, '')
						.replace(/\/$/, '')
				}))
			};
			return new Response(JSON.stringify(settings(next)), {
				status: 200,
				headers: { 'Content-Type': 'application/json' }
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

afterEach(() => {
	vi.restoreAllMocks();
	appSettingsStore.clear();
});

describe('LinkTitlesSection', () => {
	it('is off by default and hides the templates', () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [],
			inboxProcessing: { prompt: '', paused: false }
		});
		setupAuth(makeFetchMock([], { enabled: false, template: '', rules: [] }));
		render(LinkTitlesSection);

		const toggle = screen.getByRole('switch');
		expect(toggle.getAttribute('aria-checked')).toBe('false');
		expect(screen.queryByRole('button', { name: /add rule|добавить правило/i })).toBeNull();
	});

	it('switches the feature on with a partial PUT', async () => {
		const current = { enabled: false, template: '', rules: [] };
		appSettingsStore.setValue(settings(current));
		const captured: CapturedRequest[] = [];
		setupAuth(makeFetchMock(captured, current));
		render(LinkTitlesSection);

		await fireEvent.click(screen.getByRole('switch'));

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		expect(captured.find((r) => r.method === 'PUT')!.body).toEqual({ enabled: true });
		await waitFor(() => expect(appSettingsStore.linkTitles.enabled).toBe(true));
	});

	it('saves a domain rule and shows the normalised domain', async () => {
		const current = { enabled: true, template: '', rules: [] };
		appSettingsStore.setValue(settings(current));
		const captured: CapturedRequest[] = [];
		setupAuth(makeFetchMock(captured, current));
		render(LinkTitlesSection);

		await fireEvent.click(screen.getByRole('button', { name: /add rule|добавить правило/i }));
		const [domain] = screen.getAllByRole('textbox', { name: /^domain$|^домен$/i });
		const [template] = screen.getAllByRole('textbox', { name: /^template$|^шаблон$/i });
		await fireEvent.input(domain, { target: { value: 'https://www.youtube.com/' } });
		await fireEvent.input(template, { target: { value: 'Посмотреть «{title}»' } });
		await fireEvent.click(screen.getByRole('button', { name: /^save$|^сохранить$/i }));

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		expect(captured.find((r) => r.method === 'PUT')!.body).toEqual({
			template: '',
			rules: [{ domain: 'https://www.youtube.com/', template: 'Посмотреть «{title}»' }]
		});
		await waitFor(() => expect(screen.getByDisplayValue('youtube.com')).toBeTruthy());
	});

	it('refuses a rule template without {title}', async () => {
		const current = {
			enabled: true,
			template: '',
			rules: [{ domain: 'a.com', template: '{title}' }]
		};
		appSettingsStore.setValue(settings(current));
		const captured: CapturedRequest[] = [];
		setupAuth(makeFetchMock(captured, current));
		render(LinkTitlesSection);

		const [template] = screen.getAllByRole('textbox', { name: /^template$|^шаблон$/i });
		await fireEvent.input(template, { target: { value: 'no placeholder' } });
		await fireEvent.click(screen.getByRole('button', { name: /^save$|^сохранить$/i }));

		expect(captured.some((r) => r.method === 'PUT')).toBe(false);
	});
});
