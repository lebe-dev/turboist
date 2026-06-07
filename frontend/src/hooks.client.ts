import { handleErrorWithSentry } from '@sentry/sveltekit';
import { initSentry } from '$lib/observability/sentry';

// Fire-and-forget: fetch runtime config and initialise Sentry. Any error thrown
// in the brief window before init resolves simply goes unreported — acceptable
// for best-effort telemetry. Once initialised, Sentry's global handlers capture
// uncaught errors and unhandled rejections; handleErrorWithSentry below reports
// errors surfaced through SvelteKit's client navigation/render pipeline.
void initSentry();

export const handleError = handleErrorWithSentry();
