/**
 * Pure routing helpers for the web-shell service worker
 * (FEATURE-OFFLINE-ARCH.md §5.1). Kept in a dependency-free module — no
 * `$service-worker` virtual module, no ServiceWorkerGlobalScope — so they can
 * be unit-tested in jsdom, unlike `service-worker.ts` itself.
 */

/** Cache name for a given deploy version; one cache per deploy. */
export function cacheName(version: string): string {
	return `cache-${version}`;
}

/**
 * Requests the SW must NOT intercept. Everything under `/api/` — including the
 * SSE stream `GET /api/v1/events` and its ticket mint `POST
 * /api/v1/events/ticket` — and `/auth/` is left to hit the network directly:
 * offline *data* (and its caching) is exclusively the JS `lib/offline/` layer's
 * job, never the shell SW (§1, §5.1).
 */
export function shouldBypass(pathname: string): boolean {
	return pathname.startsWith('/api/') || pathname.startsWith('/auth/');
}

/** A full-page navigation (initial load / hard reload), vs a subresource fetch. */
export function isNavigationRequest(request: { mode?: string }): boolean {
	return request.mode === 'navigate';
}

/** True for any cache left over from a previous deploy (activate prunes these). */
export function isStaleCache(key: string, currentVersion: string): boolean {
	return key !== cacheName(currentVersion);
}
