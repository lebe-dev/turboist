import { render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configStore } from '$lib/stores/config.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';
import { userStateStore } from '$lib/stores/userState.svelte';
import type { ConfigResponse, Task } from '$lib/api/types';
import TodayPage from './+page.svelte';

/**
 * Regression test for the reported UX bug: open a tab that has been idle, click a
 * task's checkbox, and "the page refreshes and nothing happens" — clicking a
 * second time works.
 *
 * The mechanism: a hidden tab's throttled SSE reconnect timer fires on wake, the
 * layout invalidates every scope, and the resulting background revalidation is
 * still in flight when the user completes a task. Its payload is a snapshot of
 * server truth from BEFORE the completion, so applying it puts the task straight
 * back into the list.
 */

const { viewsToday, calendarEvents, complete } = vi.hoisted(() => ({
	viewsToday: vi.fn(),
	calendarEvents: vi.fn(async () => []),
	complete: vi.fn()
}));

vi.mock('$lib/api/client', () => ({ getApiClient: () => ({}) }));
vi.mock('$lib/api/endpoints/views', () => ({ views: { today: viewsToday } }));
vi.mock('$lib/api/endpoints/calendars', () => ({ calendars: { events: calendarEvents } }));
vi.mock('$lib/api/endpoints/tasks', () => ({
	tasks: { complete, uncomplete: vi.fn() }
}));
vi.mock('$lib/realtime/refresh', () => ({
	refreshSidebarBundle: vi.fn(async () => {}),
	refreshWorkspace: vi.fn(async () => {})
}));
vi.mock('svelte-sonner', () => ({
	toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() }
}));

function makeTask(id: number, over: Partial<Task> = {}): Task {
	return {
		id,
		title: `task-${id}`,
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
		// Due "now" so the task lands in the today bucket, never the overdue one
		// (which routes the click through a confirmation dialog instead).
		dueAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		blockedByCount: 0,
		relationCount: 0,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: '',
		...over
	};
}

/** The shape today/+page.svelte reads off the today bundle. */
function bundle(tasks: Task[]) {
	return {
		today: { items: tasks, total: tasks.length },
		overdue: { items: [], total: 0 },
		completedToday: { items: [], total: 0 }
	};
}

/**
 * The completion checkbox of the row rendering `title`. Walks up from the title
 * node to the nearest ancestor that owns a checkbox, which is that task's row.
 */
function checkboxFor(title: string): HTMLButtonElement {
	let node: HTMLElement | null = screen.getByText(title);
	while (node) {
		const btn = node.querySelector<HTMLButtonElement>('button[aria-label="Mark complete"]');
		if (btn) return btn;
		node = node.parentElement;
	}
	throw new Error(`no completion checkbox found for "${title}"`);
}

/**
 * Drains microtasks, a macrotask and Svelte's effect queue, so an assertion that
 * something did NOT render cannot pass merely by running too early.
 */
async function settled(): Promise<void> {
	await new Promise((r) => setTimeout(r, 0));
	await tick();
	await new Promise((r) => setTimeout(r, 0));
	await tick();
}

function deferred<T>() {
	let resolve!: (v: T) => void;
	const promise = new Promise<T>((r) => {
		resolve = r;
	});
	return { promise, resolve };
}

beforeEach(() => {
	configStore.value = {
		timezone: 'UTC',
		dayParts: {
			morning: { start: 6, end: 12 },
			afternoon: { start: 12, end: 18 },
			evening: { start: 18, end: 23 }
		}
	} as unknown as ConfigResponse;
	settingsStore.setValue({ ...settingsStore.value, calendarHidePastEvents: false });
	userStateStore.clear();
	viewsToday.mockReset();
	complete.mockReset();
});

afterEach(() => {
	configStore.clear();
	settingsStore.clear();
	vi.clearAllMocks();
});

describe('Today page: completing a task while a revalidation is in flight', () => {
	it('does not resurrect the completed task when the stale snapshot lands', async () => {
		const keep = makeTask(1, { title: 'keep me' });
		const done = makeTask(2, { title: 'complete me' });

		// 1) Initial load.
		viewsToday.mockResolvedValueOnce(bundle([keep, done]));
		render(TodayPage);
		expect(await screen.findByText('complete me')).toBeTruthy();

		// 2) The wake-up SSE burst kicks a background revalidation. Hold its response
		//    so it is still in flight while the user clicks — and have it return the
		//    PRE-completion list, exactly as a request issued before the click would.
		const inflight = deferred<ReturnType<typeof bundle>>();
		viewsToday.mockReturnValueOnce(inflight.promise);
		window.dispatchEvent(
			new CustomEvent('turboist:invalidate', { detail: { scope: 'tasks' } })
		);
		await waitFor(() => expect(viewsToday).toHaveBeenCalledTimes(2));

		// 3) The user completes the task; the mutation round-trips first.
		complete.mockResolvedValueOnce({ ...done, status: 'completed', completedAt: 'now' });
		checkboxFor('complete me').click();
		await waitFor(() => expect(screen.queryByText('complete me')).toBeNull());

		// 4) The stale snapshot resolves last. It must be discarded — and the retry it
		//    triggers sees post-completion truth, which is what the server now holds.
		const fresh = makeTask(3, { title: 'from another device' });
		viewsToday.mockResolvedValue(bundle([keep, fresh]));
		inflight.resolve(bundle([keep, done]));
		await settled();

		expect(screen.queryByText('complete me')).toBeNull();
		expect(screen.getByText('keep me')).toBeTruthy();
		// The discarded snapshot may also have carried a remote change, so the retry
		// must actually happen rather than leaving the view stale.
		expect(viewsToday).toHaveBeenCalledTimes(3);
		expect(screen.getByText('from another device')).toBeTruthy();
	});

	it('still applies a revalidation when the user did not touch anything', async () => {
		const keep = makeTask(1, { title: 'keep me' });
		viewsToday.mockResolvedValueOnce(bundle([keep]));
		render(TodayPage);
		expect(await screen.findByText('keep me')).toBeTruthy();

		// A genuinely remote change arrives — this one must land.
		viewsToday.mockResolvedValueOnce(bundle([keep, makeTask(2, { title: 'from another device' })]));
		window.dispatchEvent(
			new CustomEvent('turboist:invalidate', { detail: { scope: 'tasks' } })
		);
		expect(await screen.findByText('from another device')).toBeTruthy();
	});
});

// The other half of the wake-up bug: `refetch()` blanks the list behind a
// spinner, so anything that re-runs the activeContextId effect unmounts the row
// the user is clicking. A mid-session /api/v1/config refresh used to do exactly
// that on every wake, without the active context having changed at all.
describe('Today page: workspace refresh', () => {
	it('does not refetch when the server reports the same active context', async () => {
		viewsToday.mockResolvedValue(bundle([makeTask(1, { title: 'keep me' })]));
		render(TodayPage);
		expect(await screen.findByText('keep me')).toBeTruthy();
		expect(viewsToday).toHaveBeenCalledTimes(1);

		userStateStore.reconcileFromServer({ activeContextId: null });
		await settled();

		expect(viewsToday).toHaveBeenCalledTimes(1);
		expect(screen.getByText('keep me')).toBeTruthy();
	});

	it('refetches when the active context really changed', async () => {
		viewsToday.mockResolvedValue(bundle([makeTask(1, { title: 'keep me' })]));
		render(TodayPage);
		expect(await screen.findByText('keep me')).toBeTruthy();

		userStateStore.reconcileFromServer({ activeContextId: 3 });
		await settled();

		expect(viewsToday).toHaveBeenCalledTimes(2);
	});
});
