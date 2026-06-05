import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';
import { ApiError } from '$lib/api/errors';

const { toastMock } = vi.hoisted(() => ({
	toastMock: { success: vi.fn(), error: vi.fn(), info: vi.fn() }
}));
vi.mock('svelte-sonner', () => ({ toast: toastMock }));

// F3.4 — open-card "updated remotely" notice (US-3.1 AC3). A federation-origin
// change reaches the open task detail page on the `tasks` SSE scope (the
// federation publish carries no origin, so it is NOT echo-suppressed — see
// service/federation/inbox_notify.go). When the user has in-flight edits, the
// page must show a non-destructive banner instead of re-hydrating and silently
// clobbering the edit. When the editor is clean, it follows the normal F3.2
// refresh (US-3.1 AC2).

// Capture the useInvalidation callback so the test can fire a `tasks` scope
// invalidation as if it arrived over SSE.
let invalidateCb: (() => void) | null = null;
vi.mock('$lib/hooks/useInvalidation.svelte', () => ({
	useInvalidation: (_scopes: string[], cb: () => void) => {
		invalidateCb = cb;
	}
}));

function fireRemoteTasksChange(): void {
	invalidateCb?.();
}

// Page params: task id 5.
vi.mock('$app/state', () => ({
	page: { params: { id: '5' }, url: new URL('http://localhost/task/5') }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(async () => {}) }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));

let currentTask: Task;
// getError, when set, makes the NEXT tasks.get reject with it (and then clears).
// Lets a test simulate a federation rejection on an explicit Reload: peer-delete
// (410 gone), read-only (403), or expired session (401).
let getError: unknown = null;
const getTask = vi.fn(async () => {
	if (getError !== null) {
		const err = getError;
		getError = null;
		throw err;
	}
	return currentTask;
});
const updateTask = vi.fn(async () => currentTask);
vi.mock('$lib/api/endpoints/tasks', () => ({
	tasks: {
		get: (...args: unknown[]) => getTask(...(args as [])),
		update: (...args: unknown[]) => updateTask(...(args as [])),
		listSubtasks: vi.fn(async () => ({ items: [], total: 0 })),
		createSubtask: vi.fn()
	}
}));
vi.mock('$lib/api/client', () => ({ getApiClient: () => ({}) }));

// Minimal store stubs the page reads from.
vi.mock('$lib/stores/config.svelte', () => ({
	configStore: { value: { timezone: 'UTC' } }
}));
vi.mock('$lib/stores/appSettings.svelte', () => ({
	appSettingsStore: { autoLabels: [] }
}));
vi.mock('$lib/stores/labels.svelte', () => ({
	labelsStore: { favourites: [], rest: [] }
}));
vi.mock('$lib/stores/projects.svelte', () => ({
	projectsStore: { items: [] }
}));
vi.mock('$lib/stores/settings.svelte', () => ({
	settingsStore: { publicView: false, troikiEnabled: false }
}));
vi.mock('$lib/stores/now.svelte', () => ({
	nowStore: { todayKey: '2026-06-01' }
}));
vi.mock('$lib/stores/viewFilter.svelte', () => ({
	viewFilterStore: { setTitle: vi.fn(), clear: vi.fn(), title: '' }
}));
vi.mock('$lib/stores/currentTask.svelte', () => ({
	currentTaskStore: { set: vi.fn(), clear: vi.fn() }
}));

function makeTask(over: Partial<Task> = {}): Task {
	return {
		id: 5,
		title: 'Remote task',
		description: '',
		inboxId: null,
		contextId: null,
		projectId: 3,
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
		isPrivate: false,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		labels: [],
		url: '',
		federated: false,
		visibleToPeers: 0,
		clientId: 'c5',
		deletedAt: null,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...over
	};
}

const { default: TaskPage } = await import('./+page.svelte');

beforeEach(() => {
	invalidateCb = null;
	currentTask = makeTask();
	getError = null;
	getTask.mockClear();
	updateTask.mockClear();
	toastMock.error.mockClear();
});

afterEach(() => {
	vi.clearAllMocks();
});

describe('Task detail — open-card "updated remotely" notice (F3.4, US-3.1)', () => {
	it('a remote change with no in-flight edits silently refreshes — no banner (US-3.1 AC2)', async () => {
		render(TaskPage);
		await screen.findByDisplayValue('Remote task');

		const refetchCount = getTask.mock.calls.length;

		// Remote change arrives while editor is clean: server now has a new title.
		currentTask = makeTask({ title: 'Peer applied', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();

		// Normal refresh path → page re-fetched and applied the remote value.
		await waitFor(() => expect(getTask.mock.calls.length).toBeGreaterThan(refetchCount));
		await screen.findByDisplayValue('Peer applied');
		expect(screen.queryByText(/updated remotely/i)).toBeNull();
	});

	it('a remote change while editing shows a notice and does NOT overwrite the in-flight edit (US-3.1 AC3)', async () => {
		render(TaskPage);
		const titleEl = (await screen.findByDisplayValue('Remote task')) as HTMLTextAreaElement;

		// User edits the title (in-flight, unsaved).
		await fireEvent.input(titleEl, { target: { value: 'My local draft' } });

		// Remote change arrives: server has a different title now.
		currentTask = makeTask({ title: 'Peer edit', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();

		// Non-destructive notice appears...
		expect(await screen.findByText(/updated remotely/i)).toBeTruthy();
		// ...and the editor still holds the user's draft (NOT clobbered).
		expect((screen.getByDisplayValue('My local draft') as HTMLTextAreaElement).value).toBe(
			'My local draft'
		);
	});

	it('Reload pulls the latest and clears the notice', async () => {
		render(TaskPage);
		const titleEl = (await screen.findByDisplayValue('Remote task')) as HTMLTextAreaElement;
		await fireEvent.input(titleEl, { target: { value: 'My local draft' } });

		currentTask = makeTask({ title: 'Peer edit', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();
		await screen.findByText(/updated remotely/i);

		const reloadBtn = screen.getByRole('button', { name: /reload/i });
		await fireEvent.click(reloadBtn);

		// Latest pulled in; banner gone.
		await screen.findByDisplayValue('Peer edit');
		await waitFor(() => expect(screen.queryByText(/updated remotely/i)).toBeNull());
	});

	it('Reload that the peer deleted (410 gone) KEEPS the user informed — banner persists or terminal not-found, never a silent stale editor', async () => {
		render(TaskPage);
		const titleEl = (await screen.findByDisplayValue('Remote task')) as HTMLTextAreaElement;
		await fireEvent.input(titleEl, { target: { value: 'My local draft' } });

		currentTask = makeTask({ title: 'Peer edit', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();
		await screen.findByText(/updated remotely/i);

		// The peer deleted the task: the next get (the Reload) returns a 410 tombstone.
		getError = new ApiError('gone', 'task was deleted', 410);

		const reloadBtn = screen.getByRole('button', { name: /reload/i });
		await fireEvent.click(reloadBtn);

		// The user MUST be told the reload did not land on their draft. Either the
		// banner persists (still on the editor) or the page falls to the terminal
		// not-found view — what must NOT happen is a silently-cleared banner that
		// leaves the stale draft on screen as if the reload succeeded.
		await waitFor(() => {
			const notFound = screen.queryByText(/not found/i);
			const banner = screen.queryByText(/updated remotely/i);
			expect(notFound !== null || banner !== null).toBe(true);
		});
		// The draft was never overwritten by a (failed) reload.
		expect(screen.queryByDisplayValue('Peer edit')).toBeNull();
	});

	it('Reload that fails with a non-terminal error (403 read-only) surfaces the failure — never a silent success', async () => {
		render(TaskPage);
		const titleEl = (await screen.findByDisplayValue('Remote task')) as HTMLTextAreaElement;
		await fireEvent.input(titleEl, { target: { value: 'My local draft' } });

		currentTask = makeTask({ title: 'Peer edit', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();
		await screen.findByText(/updated remotely/i);

		// The project flipped read-only: the Reload fetch is rejected with 403.
		getError = new ApiError('federation_read_only', 'project is read-only', 403);

		const reloadBtn = screen.getByRole('button', { name: /reload/i });
		await fireEvent.click(reloadBtn);

		// The failure is surfaced (toasted by the loader's onError) — NOT swallowed
		// as a silent success. The user knows the reload did not land.
		await waitFor(() => expect(toastMock.error).toHaveBeenCalled());
		// And the reload never clobbered the draft with stale-as-if-success state:
		// the peer's 'Peer edit' value is nowhere on screen.
		expect(screen.queryByDisplayValue('Peer edit')).toBeNull();
	});

	it('Keep editing dismisses the notice and preserves the in-flight edit', async () => {
		render(TaskPage);
		const titleEl = (await screen.findByDisplayValue('Remote task')) as HTMLTextAreaElement;
		await fireEvent.input(titleEl, { target: { value: 'My local draft' } });

		currentTask = makeTask({ title: 'Peer edit', updatedAt: '2026-02-02T00:00:00.000Z' });
		fireRemoteTasksChange();
		await screen.findByText(/updated remotely/i);

		const keepBtn = screen.getByRole('button', { name: /keep editing/i });
		await fireEvent.click(keepBtn);

		await waitFor(() => expect(screen.queryByText(/updated remotely/i)).toBeNull());
		expect((screen.getByDisplayValue('My local draft') as HTMLTextAreaElement).value).toBe(
			'My local draft'
		);
	});
});
