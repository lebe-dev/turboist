import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, setApiClient } from '../api/client';
import { configStore } from '../stores/config.svelte';
import type { ConfigResponse, Project } from '../api/types';
import { createCacheWarmer, warmTargets } from './warmCache';

/** An ApiClient that records every URL and answers with an empty list. */
function recordingClient(calls: string[], bodyFor: (url: string) => string = () => '{"items":[]}') {
	const client = new ApiClient({
		baseUrl: '',
		getAccessToken: () => null,
		setAccessToken: () => {},
		onRefreshFailure: () => {},
		fetchImpl: (async (url: string) => {
			const u = String(url);
			calls.push(u);
			return new Response(bodyFor(u), {
				status: 200,
				headers: { 'Content-Type': 'application/json' }
			});
		}) as unknown as typeof fetch
	});
	setApiClient(client);
	return client;
}

/** Seed only the slice warmPinnedProjectBundles reads. */
function seedProjects(projects: Partial<Project>[]): void {
	configStore.value = { projects } as unknown as ConfigResponse;
}

describe('warmTargets', () => {
	afterEach(() => {
		configStore.clear();
	});

	it('issues the key-screen GETs under the cache keys the pages use', async () => {
		const calls: string[] = [];
		recordingClient(calls);
		seedProjects([]);

		await Promise.allSettled(warmTargets());

		// Paths/queries mirror what the pages request so the warmed entries land
		// under the same canonical cache key.
		expect(calls).toEqual(
			expect.arrayContaining([
				'/api/v1/tasks/today',
				'/api/v1/tasks/tomorrow',
				'/api/v1/tasks/week',
				'/api/v1/tasks/backlog',
				'/api/v1/inbox',
				'/api/v1/troiki'
			])
		);
		expect(calls).toHaveLength(6);
	});

	// These four now arrive inside /api/v1/config, which write-throughs into the
	// cache on its own. Warming their standalone paths would fill entries that
	// nothing reads.
	it('does not warm what the config aggregate already covers', async () => {
		const calls: string[] = [];
		recordingClient(calls);
		seedProjects([]);

		await Promise.allSettled(warmTargets());

		expect(calls).not.toContain('/api/v1/projects?limit=500');
		expect(calls).not.toContain('/api/v1/labels?limit=500');
		expect(calls).not.toContain('/api/v1/contexts?limit=200');
		expect(calls).not.toContain('/api/v1/tasks/pinned');
	});

	it('warms the bundle of each pinned project (and no others)', async () => {
		const calls: string[] = [];
		recordingClient(calls);
		// Read straight from the config aggregate — a bare Project[], no `.items`.
		seedProjects([{ id: 7, isPinned: true }, { id: 9, isPinned: false }]);

		await Promise.allSettled(warmTargets());

		expect(calls).toContain('/api/v1/projects/7/bundle');
		expect(calls).not.toContain('/api/v1/projects/9/bundle');
	});

	// The warmer is scheduled after configStore.load() resolves, but must not
	// throw if it somehow runs first (e.g. a retry path).
	it('warms no bundles when the config aggregate has not loaded yet', async () => {
		const calls: string[] = [];
		recordingClient(calls);
		configStore.clear();

		await Promise.allSettled(warmTargets());

		expect(calls.filter((u) => u.includes('/bundle'))).toEqual([]);
	});
});

describe('createCacheWarmer', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('debounces rapid schedule() calls into a single run', () => {
		const run = vi.fn(() => [Promise.resolve()]);
		const warmer = createCacheWarmer({ run, isOnline: () => true, debounceMs: 100 });

		warmer.schedule();
		warmer.schedule();
		warmer.schedule();
		expect(run).not.toHaveBeenCalled();

		vi.advanceTimersByTime(100);
		expect(run).toHaveBeenCalledTimes(1);
	});

	it('does not run while offline', () => {
		const run = vi.fn(() => [Promise.resolve()]);
		const warmer = createCacheWarmer({ run, isOnline: () => false, debounceMs: 100 });

		warmer.schedule();
		vi.advanceTimersByTime(100);
		expect(run).not.toHaveBeenCalled();
	});

	it('cancel() drops a pending run', () => {
		const run = vi.fn(() => [Promise.resolve()]);
		const warmer = createCacheWarmer({ run, isOnline: () => true, debounceMs: 100 });

		warmer.schedule();
		warmer.cancel();
		vi.advanceTimersByTime(100);
		expect(run).not.toHaveBeenCalled();
	});

	it('does not start a second run while one is still in flight', async () => {
		let resolveRun: () => void = () => {};
		const inflight = new Promise<void>((resolve) => {
			resolveRun = resolve;
		});
		const run = vi.fn(() => [inflight]);
		const warmer = createCacheWarmer({ run, isOnline: () => true, debounceMs: 100 });

		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(1);

		// A second cycle while the first run is unresolved is skipped.
		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(1);

		// Once the in-flight run settles, a later cycle runs again.
		resolveRun();
		await Promise.resolve();
		await Promise.resolve();

		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(2);
	});

	it('swallows a run() that throws synchronously and stays reusable', async () => {
		const run = vi.fn(() => {
			throw new Error('ApiClient not initialised');
		});
		const warmer = createCacheWarmer({ run, isOnline: () => true, debounceMs: 100 });

		warmer.schedule();
		expect(() => vi.advanceTimersByTime(100)).not.toThrow();
		expect(run).toHaveBeenCalledTimes(1);

		// The throw did not latch `running`, so a later schedule fires again.
		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(2);
	});

	it('swallows rejected warm GETs and stays reusable', async () => {
		const run = vi.fn(() => [Promise.reject(new Error('boom'))]);
		const warmer = createCacheWarmer({ run, isOnline: () => true, debounceMs: 100 });

		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(1);

		warmer.schedule();
		await vi.advanceTimersByTimeAsync(100);
		expect(run).toHaveBeenCalledTimes(2);
	});
});
