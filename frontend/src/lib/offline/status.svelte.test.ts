import { afterEach, describe, expect, it } from 'vitest';
import { createStatusStore } from './status.svelte';

// createStatusStore registers window online/offline listeners; each test builds
// its own instance and reads only that one. A shared window event flips
// navOnline on every live instance, but since a test never reads a stale
// instance this cross-talk is harmless, and a fresh instance always re-reads
// navigator.onLine at construction.
function setNavigatorOnline(value: boolean): void {
	Object.defineProperty(navigator, 'onLine', { value, configurable: true });
}

afterEach(() => {
	setNavigatorOnline(true);
});

describe('status store', () => {
	it('defaults to online with no stale/sync state before any request', () => {
		const s = createStatusStore(() => false);
		expect(s.online).toBe(true);
		expect(s.servedStale).toBe(false);
		expect(s.lastSyncAt).toBeNull();
		expect(s.syncedAt).toBeNull();
	});

	it('treats a failed request outcome as authoritative and goes offline', () => {
		const s = createStatusStore(() => false);
		s.noteOutcome(false);
		expect(s.online).toBe(false);
	});

	it('goes back online and stamps lastSyncAt on a successful request outcome', () => {
		const s = createStatusStore(() => false);
		s.noteOutcome(false);
		expect(s.online).toBe(false);
		expect(s.lastSyncAt).toBeNull();

		s.noteOutcome(true);
		expect(s.online).toBe(true);
		expect(s.lastSyncAt).toMatch(/^\d{4}-\d{2}-\d{2}T.*Z$/);
	});

	it('lets a live SSE connection force online even after a failed outcome', () => {
		let connected = false;
		const s = createStatusStore(() => connected);
		s.noteOutcome(false);
		expect(s.online).toBe(false);

		connected = true;
		expect(s.online).toBe(true);
	});

	it('lets navigator offline override a prior successful outcome', () => {
		const s = createStatusStore(() => false);
		s.noteOutcome(true);
		expect(s.online).toBe(true);

		window.dispatchEvent(new Event('offline'));
		expect(s.online).toBe(false);

		window.dispatchEvent(new Event('online'));
		expect(s.online).toBe(true);
	});

	it('seeds navOnline from navigator.onLine at construction', () => {
		setNavigatorOnline(false);
		const s = createStatusStore(() => false);
		expect(s.online).toBe(false);
	});

	it('toggles servedStale via markStale / clearStale', () => {
		const s = createStatusStore(() => false);
		expect(s.servedStale).toBe(false);

		s.markStale();
		expect(s.servedStale).toBe(true);

		s.clearStale();
		expect(s.servedStale).toBe(false);
	});

	it('emits a one-shot numeric signal from noteSynced', () => {
		const s = createStatusStore(() => false);
		expect(s.syncedAt).toBeNull();

		s.noteSynced();
		expect(typeof s.syncedAt).toBe('number');
	});

	it('mirrors setFailedOps into the failedOps getter', () => {
		const s = createStatusStore(() => false);
		expect(s.failedOps).toBe(0);

		s.setFailedOps(3);
		expect(s.failedOps).toBe(3);

		s.setFailedOps(0);
		expect(s.failedOps).toBe(0);
	});

	it('tracks pendingOps and failedOps as independent counters', () => {
		const s = createStatusStore(() => false);
		s.setPendingOps(2);
		s.setFailedOps(5);
		expect(s.pendingOps).toBe(2);
		expect(s.failedOps).toBe(5);

		// Draining the queue must not disturb the quarantined count.
		s.setPendingOps(0);
		expect(s.pendingOps).toBe(0);
		expect(s.failedOps).toBe(5);
	});
});
