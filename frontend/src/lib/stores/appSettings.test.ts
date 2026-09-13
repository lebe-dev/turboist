import { afterEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import { appSettingsStore } from './appSettings.svelte';

function setupAuth(fetchImpl: typeof fetch) {
	const store = createAuthStore({ fetchImpl });
	store.user = { id: 1, username: 'eu', totpEnabled: false };
	store.accessToken = 'A';
	store.status = 'authenticated';
	return store;
}

function json(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

afterEach(() => {
	vi.restoreAllMocks();
	appSettingsStore.clear();
});

describe('appSettingsStore.setInboxProcessingPrompt', () => {
	it('applies the prompt optimistically and keeps the server answer', async () => {
		let resolve!: (r: Response) => void;
		setupAuth(
			vi.fn(
				() =>
					new Promise<Response>((r) => {
						resolve = r;
					})
			) as unknown as typeof fetch
		);

		const pending = appSettingsStore.setInboxProcessingPrompt('Sort {{.Task.Title}}');
		expect(appSettingsStore.inboxProcessing.prompt).toBe('Sort {{.Task.Title}}');

		resolve(
			json({
				autoLabels: [],
				projectSuggestions: [],
				inboxProcessing: { prompt: 'Sort {{.Task.Title}}' }
			})
		);
		await pending;
		expect(appSettingsStore.inboxProcessing.prompt).toBe('Sort {{.Task.Title}}');
	});

	it('rolls back when the server refuses the prompt', async () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [],
			inboxProcessing: { prompt: 'previous', paused: false }
		});
		setupAuth(
			vi.fn(async () =>
				json(
					{
						error: {
							code: 'validation_failed',
							message: 'invalid prompt template',
							details: { error: 'bad' }
						}
					},
					422
				)
			) as unknown as typeof fetch
		);

		await expect(appSettingsStore.setInboxProcessingPrompt('{{.Nope}}')).rejects.toBeTruthy();
		expect(appSettingsStore.inboxProcessing.prompt).toBe('previous');
	});

	it('falls back to an empty prompt for settings payloads that predate the field', () => {
		appSettingsStore.setValue({ autoLabels: [], projectSuggestions: [] } as never);
		expect(appSettingsStore.inboxProcessing.prompt).toBe('');
	});
});

describe('appSettingsStore.setInboxProcessingPaused', () => {
	it('keeps the prompt when pausing and rolls back on failure', async () => {
		appSettingsStore.setValue({
			autoLabels: [],
			projectSuggestions: [],
			inboxProcessing: { prompt: 'mine', paused: false }
		});
		setupAuth(
			vi.fn(async () =>
				json({ error: { code: 'internal_error', message: 'boom' } }, 500)
			) as unknown as typeof fetch
		);

		const pending = appSettingsStore.setInboxProcessingPaused(true);
		expect(appSettingsStore.inboxProcessing).toEqual({ prompt: 'mine', paused: true });
		await expect(pending).rejects.toBeTruthy();
		expect(appSettingsStore.inboxProcessing).toEqual({ prompt: 'mine', paused: false });
	});
});
