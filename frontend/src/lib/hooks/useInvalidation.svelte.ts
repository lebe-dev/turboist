import { onDestroy } from 'svelte';
import type { EventScope } from '$lib/realtime/events.svelte';

/**
 * useInvalidation runs `callback` whenever the layout dispatches a
 * `turboist:invalidate` event whose scope matches one of the listed scopes.
 *
 * The layout subscribes to the SSE channel once and re-broadcasts as DOM
 * events so views work the same way for both real-time pushes and the
 * post-reconnect catch-up sweep. Bursts within a microtask coalesce into a
 * single callback invocation (e.g., a task move emits tasks+plan+inbox
 * together but the view refetches once).
 */
export function useInvalidation(scopes: EventScope[], callback: () => void): void {
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const wanted = new Set<EventScope>(scopes);
	let scheduled = false;
	function trigger(): void {
		if (scheduled) return;
		scheduled = true;
		queueMicrotask(() => {
			scheduled = false;
			callback();
		});
	}
	function handle(e: Event): void {
		const detail = (e as CustomEvent<{ scope: EventScope }>).detail;
		if (!detail || !wanted.has(detail.scope)) return;
		trigger();
	}
	window.addEventListener('turboist:invalidate', handle);
	onDestroy(() => {
		window.removeEventListener('turboist:invalidate', handle);
	});
}
