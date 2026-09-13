import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import { appSettingsStore } from '$lib/stores/appSettings.svelte';
import { projectsStore } from '$lib/stores/projects.svelte';
import type { InboxProcessingLogEntry, InboxProcessingStatus, Project } from '$lib/api/types';
import InboxProcessingSection from './InboxProcessingSection.svelte';

interface CapturedRequest {
	url: string;
	method: string;
	body: unknown;
}

const DEFAULT_PROMPT = 'You are the triage assistant.';

function status(overrides: Partial<InboxProcessingStatus> = {}): InboxProcessingStatus {
	return {
		enabled: true,
		model: 'openai/gpt-4.1-mini',
		apiHost: 'openrouter.ai',
		interval: '3m',
		batchLimit: 10,
		running: false,
		pendingCount: 2,
		lastRunAt: null,
		lastRunSummary: null,
		lastError: null,
		backoffUntil: null,
		paused: false,
		defaultPrompt: DEFAULT_PROMPT,
		...overrides
	};
}

function sortedEntry(): InboxProcessingLogEntry {
	return {
		id: 11,
		taskId: 42,
		taskTitle: 'Plant tulips',
		outcome: 'sorted',
		model: 'openai/gpt-4.1-mini',
		reason: 'garden work',
		confidence: 0.9,
		before: { labelIds: [], priority: 'no-priority', dueAt: null, dueHasTime: false },
		after: { contextId: 1, projectId: 4, labelIds: [], priority: 'no-priority', dueAt: null },
		error: null,
		promptTokens: 100,
		completionTokens: 20,
		revertedAt: null,
		createdAt: '2026-09-13T10:00:00.000Z'
	};
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

function json(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

interface MockOptions {
	statuses: InboxProcessingStatus[];
	log?: InboxProcessingLogEntry[];
	putStatus?: number;
}

function makeFetchMock(captured: CapturedRequest[], opts: MockOptions): typeof fetch {
	let statusIndex = 0;
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

		if (method === 'GET' && url.includes('/api/v1/inbox/processing/log')) {
			const items = opts.log ?? [];
			return json({ items, total: items.length, limit: 50, offset: 0 });
		}
		if (method === 'GET' && url.endsWith('/api/v1/inbox/processing')) {
			const s = opts.statuses[Math.min(statusIndex, opts.statuses.length - 1)];
			statusIndex++;
			return json(s);
		}
		if (method === 'POST' && url.endsWith('/api/v1/inbox/processing/run')) {
			return json({ running: true }, 202);
		}
		if (method === 'POST' && url.endsWith('/api/v1/inbox/processing/preview')) {
			return json({ rendered: 'RENDERED PROMPT' });
		}
		if (
			method === 'POST' &&
			url.includes('/api/v1/inbox/processing/log/') &&
			url.endsWith('/revert')
		) {
			return json({ id: 42, inboxId: 1, autoSortedAt: null });
		}
		if (method === 'PUT' && url.endsWith('/api/v1/app-settings/inbox-processing')) {
			if (opts.putStatus === 422) {
				return json(
					{
						error: {
							code: 'validation_failed',
							message: 'invalid prompt template',
							details: { error: "can't evaluate field Projcets" }
						}
					},
					422
				);
			}
			const requested = body as { prompt?: string; paused?: boolean };
			return json({
				autoLabels: [],
				projectSuggestions: [],
				inboxProcessing: {
					prompt: requested.prompt ?? appSettingsStore.inboxProcessing.prompt,
					paused: requested.paused ?? appSettingsStore.inboxProcessing.paused
				}
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

function textarea(): HTMLTextAreaElement {
	return screen.getByRole('textbox', { name: /^prompt$|^промпт$/i }) as HTMLTextAreaElement;
}

afterEach(() => {
	vi.restoreAllMocks();
	appSettingsStore.clear();
	projectsStore.clear();
});

describe('InboxProcessingSection', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
		projectsStore.setItems([project(4, 'Garden')]);
	});

	it('renders the status of an enabled processor', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });

		await waitFor(() => expect(screen.getByText('openai/gpt-4.1-mini')).toBeTruthy());
		expect(screen.getByTestId('inbox-processing-state').textContent).toMatch(/enabled|включено/i);
		expect(screen.getByTestId('inbox-processing-pending').textContent).toBe('2');
		expect(screen.queryByText(/INBOX_PROCESSING_ENABLED=true/)).toBeNull();
		// The default prompt is what the editor shows while nothing custom is saved.
		await waitFor(() => expect(textarea().value).toBe(DEFAULT_PROMPT));
	});

	it('explains how to enable a disabled processor and disables the run button', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status({ enabled: false, model: '' })] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });

		await waitFor(() => expect(screen.getByText(/INBOX_PROCESSING_ENABLED=true/)).toBeTruthy());
		const run = screen.getByRole('button', {
			name: /process now|разобрать сейчас/i
		}) as HTMLButtonElement;
		expect(run.disabled).toBe(true);
	});

	it('saves an edited prompt', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });
		await waitFor(() => expect(textarea().value).toBe(DEFAULT_PROMPT));

		await fireEvent.input(textarea(), { target: { value: 'Sort {{.Task.Title}}' } });
		await fireEvent.click(screen.getByRole('button', { name: /^save$|^сохранить$/i }));

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		expect(captured.find((r) => r.method === 'PUT')!.body).toEqual({
			prompt: 'Sort {{.Task.Title}}'
		});
		await waitFor(() =>
			expect(appSettingsStore.inboxProcessing.prompt).toBe('Sort {{.Task.Title}}')
		);
	});

	it('resets to the default by saving an empty prompt', async () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [],
			inboxProcessing: { prompt: 'custom prompt', paused: false }
		});
		setupAuth(makeFetchMock(captured, { statuses: [status()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });
		await waitFor(() => expect(textarea().value).toBe('custom prompt'));

		await fireEvent.click(
			screen.getByRole('button', { name: /reset to default|сбросить к стандартному/i })
		);

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		expect(captured.find((r) => r.method === 'PUT')!.body).toEqual({ prompt: '' });
		await waitFor(() => expect(textarea().value).toBe(DEFAULT_PROMPT));
	});

	it('shows a template error returned by the server under the field', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()], putStatus: 422 }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });
		await waitFor(() => expect(textarea().value).toBe(DEFAULT_PROMPT));

		await fireEvent.input(textarea(), { target: { value: '{{range .Projcets}}{{end}}' } });
		await fireEvent.click(screen.getByRole('button', { name: /^save$|^сохранить$/i }));

		await waitFor(() =>
			expect(screen.getByTestId('inbox-processing-template-error').textContent).toContain(
				'Projcets'
			)
		);
		expect(appSettingsStore.inboxProcessing.prompt).toBe('');
	});

	it('previews the prompt being edited', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });
		await waitFor(() => expect(textarea().value).toBe(DEFAULT_PROMPT));

		await fireEvent.input(textarea(), { target: { value: 'Draft {{.Now}}' } });
		await fireEvent.click(screen.getByRole('button', { name: /^preview$|^предпросмотр$/i }));

		await waitFor(() => expect(captured.some((r) => r.url.endsWith('/preview'))).toBe(true));
		expect(captured.find((r) => r.url.endsWith('/preview'))!.body).toEqual({
			prompt: 'Draft {{.Now}}'
		});
		await waitFor(() =>
			expect(screen.getByTestId('inbox-processing-preview').textContent).toBe('RENDERED PROMPT')
		);
	});

	it('runs now, polls the status until the run ends and reloads the journal', async () => {
		setupAuth(
			makeFetchMock(captured, {
				statuses: [
					status(),
					status({ running: true }),
					status({
						running: false,
						pendingCount: 0,
						lastRunAt: '2026-09-13T10:00:00.000Z',
						lastRunSummary: { sorted: 2, kept: 0, failed: 0 }
					})
				]
			})
		);
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });
		const run = await waitFor(() => {
			const b = screen.getByRole('button', {
				name: /process now|разобрать сейчас/i
			}) as HTMLButtonElement;
			expect(b.disabled).toBe(false);
			return b;
		});
		const logLoadsBefore = captured.filter((r) => r.url.includes('/inbox/processing/log')).length;

		await fireEvent.click(run);

		await waitFor(() =>
			expect(captured.some((r) => r.url.endsWith('/run') && r.method === 'POST')).toBe(true)
		);
		await waitFor(() =>
			expect(
				captured.filter((r) => r.method === 'GET' && r.url.endsWith('/inbox/processing')).length
			).toBe(3)
		);
		await waitFor(() =>
			expect(
				captured.filter((r) => r.url.includes('/inbox/processing/log')).length
			).toBeGreaterThan(logLoadsBefore)
		);
		await waitFor(() =>
			expect(screen.getByTestId('inbox-processing-pending').textContent).toBe('0')
		);
	});

	it('returns a filed task to the Inbox from the journal', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()], log: [sortedEntry()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });

		await waitFor(() => expect(screen.getByText('Plant tulips')).toBeTruthy());
		expect(screen.getByText('Garden')).toBeTruthy();
		await fireEvent.click(
			screen.getByRole('button', { name: /return to inbox|вернуть во входящие/i })
		);

		await waitFor(() =>
			expect(
				captured.some(
					(r) => r.method === 'POST' && r.url.endsWith('/inbox/processing/log/11/revert')
				)
			).toBe(true)
		);
	});
});

describe('InboxProcessingSection pause', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
	});

	it('pauses automatic processing through the settings endpoint', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status()] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });

		const toggle = await waitFor(() =>
			screen.getByRole('switch', {
				name: /pause automatic processing|приостановить автоматический разбор/i
			})
		);
		await fireEvent.click(toggle);

		await waitFor(() => expect(captured.some((r) => r.method === 'PUT')).toBe(true));
		expect(captured.find((r) => r.method === 'PUT')!.body).toEqual({ paused: true });
		await waitFor(() => expect(appSettingsStore.inboxProcessing.paused).toBe(true));
		expect(screen.getByTestId('inbox-processing-state').textContent).toMatch(/paused|на паузе/i);
	});

	it('offers no pause switch while the feature is disabled', async () => {
		setupAuth(makeFetchMock(captured, { statuses: [status({ enabled: false })] }));
		render(InboxProcessingSection, { props: { pollIntervalMs: 5 } });

		await waitFor(() => expect(screen.getByText(/INBOX_PROCESSING_ENABLED=true/)).toBeTruthy());
		expect(screen.queryByRole('switch')).toBeNull();
	});
});
