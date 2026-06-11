import * as Sentry from '@sentry/sveltekit';

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

	let config: RuntimeConfig;
	try {
		const res = await fetch('/api/config', { headers: { accept: 'application/json' } });
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
		tracesSampleRate: 0
	});
	initialized = true;
}
