import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import { pinnedStore } from '$lib/stores/pinned.svelte';

vi.mock('svelte-intl-precompile', () => ({
	t: {
		subscribe(fn: (value: any) => void) {
			fn((key: string) => key);
			return () => {};
		}
	}
}));

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	patchState: vi.fn(() => Promise.resolve()),
	getAppConfig: vi.fn(),
	getCompletedTasks: vi.fn(),
	deleteTask: vi.fn()
}));

vi.mock('$lib/i18n', () => ({
	applyLocaleFromConfig: vi.fn(),
	availableLocales: ['en']
}));

vi.mock('$lib/ws/client.svelte', () => ({
	wsClient: {
		subscribe: vi.fn(),
		unsubscribe: vi.fn(),
		send: vi.fn(),
		onMessage: vi.fn(),
		connected: false
	}
}));

import ContextSwitcher from './ContextSwitcher.svelte';

describe('ContextSwitcher pinned sort', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		pinnedStore.init([], 10);
	});

	afterEach(() => {
		cleanup();
	});

	it('renders pinned tasks sorted by priority descending (P1 first)', () => {
		pinnedStore.init(
			[
				{ id: 'low', content: 'Low priority', priority: 1 },
				{ id: 'high', content: 'High priority', priority: 4 },
				{ id: 'med', content: 'Med priority', priority: 3 },
				{ id: 'none', content: 'No priority' }
			],
			10
		);

		render(ContextSwitcher);

		const links = screen.getAllByRole('link').filter((el) => el.getAttribute('href')?.startsWith('/task/'));
		const hrefs = links.map((el) => el.getAttribute('href'));

		// Priority 4 (P1) first, then 3 (P2), then 1 (P4), then undefined (treated as 1)
		expect(hrefs).toEqual(['/task/high', '/task/med', '/task/low', '/task/none']);
	});

	it('renders pinned tasks with equal priority in original order', () => {
		pinnedStore.init(
			[
				{ id: 'a', content: 'Task A', priority: 2 },
				{ id: 'b', content: 'Task B', priority: 2 },
				{ id: 'c', content: 'Task C', priority: 2 }
			],
			10
		);

		render(ContextSwitcher);

		const links = screen.getAllByRole('link').filter((el) => el.getAttribute('href')?.startsWith('/task/'));
		const hrefs = links.map((el) => el.getAttribute('href'));

		expect(hrefs).toEqual(['/task/a', '/task/b', '/task/c']);
	});
});
