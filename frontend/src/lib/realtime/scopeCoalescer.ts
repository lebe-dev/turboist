import type { EventScope } from './events.svelte';

/** Coalescing window: scopes arriving within this many ms flush together. */
export const COALESCE_MS = 200;

export interface ScopeCoalescer {
	/** Record a scope; schedules a flush if one is not already pending. */
	add(scope: EventScope): void;
	/** Drop a pending flush without running it (teardown). */
	cancel(): void;
}

export interface ScopeCoalescerOptions {
	/** Called once per window with every scope recorded during it. */
	flush: (scopes: Set<EventScope>) => void;
	/** Window length in ms. Defaults to COALESCE_MS. */
	windowMs?: number;
}

/**
 * Collect change scopes over a short window and hand them to `flush` in one go.
 *
 * A single user action or a single remote bulk operation fans out to several
 * scopes (a task edit touches `tasks`, `plan` and `inbox`), and each one used to
 * trigger its own GET. Coalescing lets the flush handler pick ONE aggregate
 * request that covers the whole set — which on mobile is the difference between
 * one radio wake-up and six.
 *
 * The window opens on the first scope after an idle period and is not extended
 * by later arrivals, so a steady stream still flushes every `windowMs` rather
 * than being starved. A scope recorded while a flush is running lands in the
 * next window.
 */
export function createScopeCoalescer(options: ScopeCoalescerOptions): ScopeCoalescer {
	const windowMs = options.windowMs ?? COALESCE_MS;
	const pending = new Set<EventScope>();
	let timer: ReturnType<typeof setTimeout> | null = null;

	function flush(): void {
		timer = null;
		const scopes = new Set(pending);
		pending.clear();
		if (scopes.size === 0) return;
		options.flush(scopes);
	}

	return {
		add(scope: EventScope): void {
			pending.add(scope);
			if (timer !== null) return;
			timer = setTimeout(flush, windowMs);
		},
		cancel(): void {
			if (timer !== null) {
				clearTimeout(timer);
				timer = null;
			}
			pending.clear();
		}
	};
}
