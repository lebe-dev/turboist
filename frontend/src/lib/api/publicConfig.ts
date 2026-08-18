import { getServerUrl } from '$lib/native/serverUrl';

/**
 * The unauthenticated `GET /api/config` payload.
 *
 * Read before login (the SPA has no token yet), so it carries only what the
 * login screen and bootstrap need: the Sentry DSN and whether a passkey sign-in
 * would lead anywhere on this instance.
 */
export interface PublicConfig {
	sentry?: { dsn?: string; environment?: string };
	passkeys?: { enabled: boolean; available: boolean };
}

let inflight: Promise<PublicConfig | null> | null = null;

/**
 * Fetch the public config once per page load. A failure resolves to null rather
 * than throwing: every caller treats it as "feature off", and none of them are
 * important enough to block a login on.
 */
export function loadPublicConfig(): Promise<PublicConfig | null> {
	inflight ??= fetchConfig();
	return inflight;
}

async function fetchConfig(): Promise<PublicConfig | null> {
	try {
		const res = await fetch(`${getServerUrl()}/api/config`, {
			headers: { accept: 'application/json' }
		});
		if (!res.ok) return null;
		return (await res.json()) as PublicConfig;
	} catch {
		return null;
	}
}
