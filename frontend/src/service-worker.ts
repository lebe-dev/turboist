/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true" />
/// <reference lib="esnext" />
/// <reference lib="webworker" />

import { base, build, files, version } from '$service-worker';
import { cacheName, isNavigationRequest, isStaleCache, shouldBypass } from '$lib/sw/routing';

// Web-shell service worker (FEATURE-OFFLINE-ARCH.md §5.1). SvelteKit
// auto-registers this file whenever it exists, so there is no explicit
// `navigator.serviceWorker.register` anywhere in the app. It ships ONLY the
// static shell — the precached app bundle plus everything under `static/`. It
// never touches `/api/*` or `/auth/*`: offline *data* is exclusively the JS
// `lib/offline/` layer's job (§1, §5.1). On native (Capacitor / WKWebView)
// `navigator.serviceWorker` is undefined, so SvelteKit's registration silently
// no-ops and none of this ever runs — no platform guard needed.

const sw = self as unknown as ServiceWorkerGlobalScope;

const CACHE = cacheName(version);

// The immutable app bundle (`build`) plus every static asset (`files`). No API
// response ever enters this set — there is no runtime API caching in the SW.
const PRECACHE = [...build, ...files];
const PRECACHE_SET = new Set(PRECACHE);

// SPA shell: adapter-static serves `index.html` (svelte.config.js
// `fallback: 'index.html'`) for every non-prerendered route, so a cached copy
// of the base URL is what offline navigations fall back to.
const SHELL_URL = `${base}/`;

sw.addEventListener('install', (event) => {
	event.waitUntil(
		(async () => {
			const cache = await caches.open(CACHE);
			await cache.addAll(PRECACHE);
			// Precache the SPA shell under a stable key so hard reloads / cold
			// starts render offline. Install runs on the first (online)
			// registration, so this fetch normally succeeds; if it can't, the shell
			// is simply unavailable until the next online visit.
			try {
				const shell = await fetch(SHELL_URL, { cache: 'no-store' });
				if (shell.ok) await cache.put(SHELL_URL, shell.clone());
			} catch {
				// Offline at install time — non-fatal.
			}
			await sw.skipWaiting();
		})()
	);
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		(async () => {
			// A version bump changes CACHE, so every other cache is a stale deploy.
			for (const key of await caches.keys()) {
				if (isStaleCache(key, version)) await caches.delete(key);
			}
			await sw.clients.claim();
		})()
	);
});

sw.addEventListener('fetch', (event) => {
	const { request } = event;
	// Only GETs are cacheable; leave everything else to the network.
	if (request.method !== 'GET') return;

	const url = new URL(request.url);
	// Cross-origin (e.g. a CDN) — not ours to serve.
	if (url.origin !== sw.location.origin) return;
	// /api/* (incl. the SSE stream) and /auth/* must reach the network untouched.
	if (shouldBypass(url.pathname)) return;

	// Full-page navigations: network-first, falling back to the cached shell so
	// the SPA still boots offline (mirrors fallback: 'index.html').
	if (isNavigationRequest(request)) {
		event.respondWith(networkFirstShell(request));
		return;
	}

	// Precached bundle/static assets are immutable-hashed → cache-first.
	if (PRECACHE_SET.has(url.pathname)) {
		event.respondWith(cacheFirst(request));
		return;
	}

	// Anything else same-origin (not precached, not a navigation) falls through
	// to the network — no runtime caching in the SW (§5.1).
});

async function networkFirstShell(request: Request): Promise<Response> {
	try {
		return await fetch(request);
	} catch {
		const cache = await caches.open(CACHE);
		const cached = (await cache.match(SHELL_URL)) ?? (await cache.match(request));
		if (cached) return cached;
		return Response.error();
	}
}

async function cacheFirst(request: Request): Promise<Response> {
	const cache = await caches.open(CACHE);
	const cached = await cache.match(request);
	if (cached) return cached;
	// Cache miss (an asset outside the precache manifest, or a first-seen hash):
	// fetch and, on success, populate the versioned cache.
	const response = await fetch(request);
	if (response.ok) await cache.put(request, response.clone());
	return response;
}
