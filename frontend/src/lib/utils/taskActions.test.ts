import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';
import { ApiClient, setApiClient } from '$lib/api/client';

vi.mock('svelte-sonner', () => ({
	toast: { error: vi.fn(), success: vi.fn() }
}));

vi.mock('$lib/stores/troiki.svelte', () => ({
	troikiStore: { applyTaskUpdate: vi.fn(), removeTask: vi.fn(), clear: vi.fn() }
}));

import { setTroikiCategory } from './taskActions';
import { toast } from 'svelte-sonner';
import { troikiStore } from '$lib/stores/troiki.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function makeTask(over: Partial<Task> = {}): Task {
	return {
		id: 1,
		title: 't',
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		troikiCategory: null,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: '',
		...over
	};
}

function installClient(fetchMock: ReturnType<typeof vi.fn>) {
	const client = new ApiClient({
		fetchImpl: fetchMock as unknown as typeof fetch,
		getAccessToken: () => null,
		setAccessToken: () => {},
		onRefreshFailure: () => {}
	});
	setApiClient(client);
}

describe('setTroikiCategory', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});
	afterEach(() => vi.restoreAllMocks());

	it('updates task and applies to mutator + troiki store on success', async () => {
		const updated = makeTask({ troikiCategory: 'important' });
		const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(jsonResponse(updated));
		installClient(fetchMock);

		const replace = vi.fn();
		const remove = vi.fn();
		await setTroikiCategory(makeTask(), 'important', { replace, remove });

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/tasks/1/troiki');
		expect((init as RequestInit).method).toBe('POST');
		expect((init as RequestInit).body).toBe(JSON.stringify({ category: 'important' }));

		expect(replace).toHaveBeenCalledWith(updated);
		expect(remove).not.toHaveBeenCalled();
		expect(troikiStore.applyTaskUpdate).toHaveBeenCalledWith(updated);
		expect(toast.error).not.toHaveBeenCalled();
	});

	it('passes null body when clearing the category', async () => {
		const updated = makeTask({ troikiCategory: null });
		const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(jsonResponse(updated));
		installClient(fetchMock);

		await setTroikiCategory(makeTask({ troikiCategory: 'medium' }), null, {
			replace: vi.fn(),
			remove: vi.fn()
		});

		expect((fetchMock.mock.calls[0][1] as RequestInit).body).toBe(JSON.stringify({ category: null }));
	});

	it('removes task from current view when belongs() returns false', async () => {
		const updated = makeTask({ troikiCategory: 'rest' });
		const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(jsonResponse(updated));
		installClient(fetchMock);

		const replace = vi.fn();
		const remove = vi.fn();
		await setTroikiCategory(makeTask(), 'rest', { replace, remove }, { belongs: () => false });

		expect(remove).toHaveBeenCalledWith(updated.id);
		expect(replace).not.toHaveBeenCalled();
	});

	it('shows specific toast and does not call mutator on troiki_slot_full', async () => {
		const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(
			jsonResponse(
				{ error: { code: 'troiki_slot_full', message: 'Important slot is full' } },
				409
			)
		);
		installClient(fetchMock);

		const replace = vi.fn();
		const remove = vi.fn();
		await setTroikiCategory(makeTask(), 'important', { replace, remove });

		expect(replace).not.toHaveBeenCalled();
		expect(remove).not.toHaveBeenCalled();
		expect(troikiStore.applyTaskUpdate).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalledWith('Important slot is full');
	});

	it('falls back to default error toast for non-slot errors', async () => {
		const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(
			jsonResponse({ error: { code: 'not_found', message: '' } }, 404)
		);
		installClient(fetchMock);

		await setTroikiCategory(makeTask(), 'important', { replace: vi.fn(), remove: vi.fn() });

		expect(toast.error).toHaveBeenCalledTimes(1);
		const arg = (toast.error as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0];
		expect(typeof arg).toBe('string');
	});

	it('returns early without calling api when category is unchanged', async () => {
		const fetchMock = vi.fn<typeof fetch>();
		installClient(fetchMock);

		await setTroikiCategory(makeTask({ troikiCategory: 'medium' }), 'medium', {
			replace: vi.fn(),
			remove: vi.fn()
		});

		expect(fetchMock).not.toHaveBeenCalled();
		expect(troikiStore.applyTaskUpdate).not.toHaveBeenCalled();
	});
});
