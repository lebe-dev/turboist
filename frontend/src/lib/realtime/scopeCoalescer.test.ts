import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { COALESCE_MS, createScopeCoalescer } from './scopeCoalescer';
import type { EventScope } from './events.svelte';

describe('createScopeCoalescer', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	// The motivating case: a remote bulk move emits several scopes at once, and
	// each one used to trigger its own GET.
	it('collapses a burst into one flush carrying every scope', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		const burst: EventScope[] = ['tasks', 'plan', 'inbox', 'projects', 'labels', 'contexts'];
		for (const s of burst) c.add(s);

		expect(flush).not.toHaveBeenCalled();
		vi.advanceTimersByTime(COALESCE_MS);

		expect(flush).toHaveBeenCalledTimes(1);
		expect([...flush.mock.calls[0][0]].sort()).toEqual([...burst].sort());
	});

	it('deduplicates a scope repeated within the window', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		c.add('tasks');
		c.add('tasks');
		c.add('tasks');
		vi.advanceTimersByTime(COALESCE_MS);

		expect(flush).toHaveBeenCalledTimes(1);
		expect([...flush.mock.calls[0][0]]).toEqual(['tasks']);
	});

	// The window is not extended by later arrivals, so a steady stream of events
	// still flushes on schedule instead of being starved indefinitely.
	it('does not let a continuing stream postpone the flush', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		c.add('tasks');
		vi.advanceTimersByTime(COALESCE_MS - 1);
		c.add('plan');
		vi.advanceTimersByTime(1);

		expect(flush).toHaveBeenCalledTimes(1);
		expect([...flush.mock.calls[0][0]].sort()).toEqual(['plan', 'tasks']);
	});

	it('opens a fresh window for scopes arriving after a flush', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		c.add('tasks');
		vi.advanceTimersByTime(COALESCE_MS);
		c.add('labels');
		vi.advanceTimersByTime(COALESCE_MS);

		expect(flush).toHaveBeenCalledTimes(2);
		expect([...flush.mock.calls[1][0]]).toEqual(['labels']);
	});

	// Teardown: the layout cancels on unmount, and a flush firing afterwards
	// would refetch into a destroyed component tree.
	it('cancel() drops a pending flush', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		c.add('tasks');
		c.cancel();
		vi.advanceTimersByTime(COALESCE_MS * 5);

		expect(flush).not.toHaveBeenCalled();
	});

	it('cancel() clears pending scopes so they do not leak into a later window', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush });

		c.add('tasks');
		c.cancel();
		c.add('labels');
		vi.advanceTimersByTime(COALESCE_MS);

		expect(flush).toHaveBeenCalledTimes(1);
		expect([...flush.mock.calls[0][0]]).toEqual(['labels']);
	});

	it('honours a custom window', () => {
		const flush = vi.fn();
		const c = createScopeCoalescer({ flush, windowMs: 50 });

		c.add('tasks');
		vi.advanceTimersByTime(49);
		expect(flush).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(flush).toHaveBeenCalledTimes(1);
	});
});
