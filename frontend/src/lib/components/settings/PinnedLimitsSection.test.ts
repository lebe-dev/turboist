import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthStore } from '$lib/auth/store.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';
import type { UserSettings } from '$lib/api/types';
import PinnedLimitsSection from './PinnedLimitsSection.svelte';

interface CapturedRequest {
	url: string;
	method: string;
	body: unknown;
}

function userSettings(over: Partial<UserSettings> = {}): UserSettings {
	return {
		weeklyUnplannedExcludedLabelIds: [],
		bugLabelIds: [],
		locale: 'en',
		publicView: false,
		bannerText: '',
		bannerPublished: false,
		bannerDayPart: '',
		calendarEnabled: false,
		calendarHidePastEvents: true,
		troikiEnabled: false,
		maxPinnedTasks: 10,
		maxPinnedProjects: 10,
		...over
	};
}

function makeFetchMock(
	captured: CapturedRequest[],
	status = 200,
	server: UserSettings = userSettings()
): typeof fetch {
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

		if (url.endsWith('/api/v1/settings') && method === 'PATCH') {
			if (status !== 200) {
				return new Response(
					JSON.stringify({ error: { code: 'validation', message: 'out of range' } }),
					{ status, headers: { 'Content-Type': 'application/json' } }
				);
			}
			server = { ...server, ...(body as Partial<UserSettings>) };
			return new Response(JSON.stringify(server), {
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

const saveButton = () => screen.getByRole('button', { name: /^save$|^сохранить$/i });

afterEach(() => {
	vi.restoreAllMocks();
	settingsStore.clear();
});

describe('PinnedLimitsSection', () => {
	let captured: CapturedRequest[];

	beforeEach(() => {
		captured = [];
	});

	it('shows the current caps and keeps Save disabled until something changes', () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 4, maxPinnedProjects: 12 }));
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);

		expect(screen.getByDisplayValue('4')).toBeTruthy();
		expect(screen.getByDisplayValue('12')).toBeTruthy();
		expect((saveButton() as HTMLButtonElement).disabled).toBe(true);
	});

	it('patches only the changed cap', async () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 10 }));
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);

		await fireEvent.input(screen.getAllByRole('spinbutton')[0], { target: { value: '3' } });
		await fireEvent.click(saveButton());

		await waitFor(() => expect(captured.some((r) => r.method === 'PATCH')).toBe(true));
		const patches = captured.filter((r) => r.method === 'PATCH');
		expect(patches).toHaveLength(1);
		expect(patches[0].body).toEqual({ maxPinnedTasks: 3 });
		expect(settingsStore.maxPinnedTasks).toBe(3);
	});

	it('patches both caps when both change', async () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 10 }));
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);

		const [tasks, projects] = screen.getAllByRole('spinbutton');
		await fireEvent.input(tasks, { target: { value: '2' } });
		await fireEvent.input(projects, { target: { value: '50' } });
		await fireEvent.click(saveButton());

		await waitFor(() => expect(captured.filter((r) => r.method === 'PATCH')).toHaveLength(2));
		expect(captured.filter((r) => r.method === 'PATCH').map((r) => r.body)).toEqual([
			{ maxPinnedTasks: 2 },
			{ maxPinnedProjects: 50 }
		]);
	});

	it('does not send an out-of-range or empty value', async () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 10 }));
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);

		const [tasks] = screen.getAllByRole('spinbutton');
		for (const value of ['0', '51', '']) {
			await fireEvent.input(tasks, { target: { value } });
			expect((saveButton() as HTMLButtonElement).disabled).toBe(true);
		}
		expect(captured.filter((r) => r.method === 'PATCH')).toHaveLength(0);
	});

	it('picks up caps that arrive after mount', async () => {
		// The section can mount before configStore.load() resolves; until then the
		// store holds the EMPTY defaults.
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);
		expect(screen.getAllByDisplayValue('10')).toHaveLength(2);

		settingsStore.setValue(userSettings({ maxPinnedTasks: 6, maxPinnedProjects: 20 }));

		await waitFor(() => expect(screen.getByDisplayValue('6')).toBeTruthy());
		expect(screen.getByDisplayValue('20')).toBeTruthy();
		expect((saveButton() as HTMLButtonElement).disabled).toBe(true);
	});

	it('keeps a half-typed value when the store changes underneath', async () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 10 }));
		setupAuth(makeFetchMock(captured));
		render(PinnedLimitsSection);

		const [tasks] = screen.getAllByRole('spinbutton');
		await fireEvent.input(tasks, { target: { value: '5' } });
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 30 }));

		await waitFor(() => expect(screen.getByDisplayValue('30')).toBeTruthy());
		expect((tasks as HTMLInputElement).value).toBe('5');
	});

	it('reverts the drafts when the server rejects the patch', async () => {
		settingsStore.setValue(userSettings({ maxPinnedTasks: 10, maxPinnedProjects: 10 }));
		setupAuth(makeFetchMock(captured, 400));
		render(PinnedLimitsSection);

		await fireEvent.input(screen.getAllByRole('spinbutton')[0], { target: { value: '7' } });
		await fireEvent.click(saveButton());

		await waitFor(() => expect((saveButton() as HTMLButtonElement).disabled).toBe(true));
		expect(screen.getByDisplayValue('10')).toBeTruthy();
	});
});
