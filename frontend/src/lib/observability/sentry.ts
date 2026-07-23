import * as Sentry from '@sentry/sveltekit';
import { isNativePlatform } from '$lib/native/platform';
import { getServerUrl } from '$lib/native/serverUrl';
import { ApiError } from '$lib/api/errors';

interface RuntimeConfig {
	sentry?: {
		dsn?: string;
		environment?: string;
	};
}

let initialized = false;

/**
 * Initialise Sentry from runtime configuration served by the backend.
 *
 * The DSN is intentionally NOT baked into the static bundle: we fetch it from
 * the public `GET /api/config` endpoint at startup so the same build works
 * across deploys (and so Sentry can be toggled without rebuilding the SPA). A
 * blank DSN — or an unreachable config endpoint — leaves Sentry disabled; that
 * is non-fatal, telemetry is best-effort and must never block the app.
 */
export async function initSentry(): Promise<void> {
	if (initialized) return;

	// Native: nothing to fetch until the server URL is entered on first launch.
	// +layout.svelte re-calls initSentry() once the URL is known. Web keeps the
	// same-origin path ('' + '/api/config').
	if (isNativePlatform() && !getServerUrl()) return;

	let config: RuntimeConfig;
	try {
		const res = await fetch(`${getServerUrl()}/api/config`, { headers: { accept: 'application/json' } });
		if (!res.ok) {
			console.warn(`Sentry config unavailable: /api/config returned ${res.status}`);
			return;
		}
		config = (await res.json()) as RuntimeConfig;
	} catch (err) {
		console.warn('Sentry config fetch failed; running without error reporting', err);
		return;
	}

	const dsn = config.sentry?.dsn;
	if (!dsn) return;

	Sentry.init({
		dsn,
		environment: config.sentry?.environment || undefined,
		release: __APP_VERSION__,
		// Errors only — no performance tracing.
		tracesSampleRate: 0,
		// Stale-chunk errors after a deploy: the client still references old
		// hashed asset URLs that no longer exist on the server. Not actionable —
		// the preloadError handler in +layout.svelte reloads the page to recover.
		ignoreErrors: [
			/Failed to fetch dynamically imported module/i,
			/Importing a module script failed/i,
			/error loading dynamically imported module/i
		],
		// Offline degradation (lib/offline/) is expected behavior, not a bug to
		// track — and attempting the report would itself just fail the same way
		// (ERR_INTERNET_DISCONNECTED), adding noise for nothing. ApiClient wraps
		// every failed fetch as ApiError('network_error', …, 0), so drop those
		// here rather than at each call site.
		beforeSend(event, hint) {
			const err = hint?.originalException;
			if (err instanceof ApiError && err.code === 'network_error') return null;
			return event;
		}
	});
	initialized = true;
}
