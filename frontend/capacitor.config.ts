import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
	appId: 'ru.tinyops.turboist',
	appName: 'Turboist',
	// SvelteKit adapter-static output (relative to this file). `yarn build` must
	// run before `cap add`/`cap sync`.
	webDir: 'build',

	// The scheme is LEFT AT ITS DEFAULT on purpose: iosScheme 'capacitor' gives the
	// origin capacitor://localhost, a secure context, so crypto.randomUUID()
	// (lib/realtime/origin.ts) works.

	plugins: {
		// Patch window.fetch + XMLHttpRequest to the native HTTP stack. Every REST
		// call then goes through the native bridge and bypasses WebView CORS, so
		// the backend needs CORS ONLY for the SSE EventSource (which CapacitorHttp
		// does NOT patch — see internal/httpapi/handlers/events.go).
		CapacitorHttp: { enabled: true },

		// CapacitorCookies is intentionally left disabled: native auth uses body
		// refresh tokens persisted in the Keychain, not the web HttpOnly
		// cookie (the backend sets that cookie only for clientKind 'web').

		SplashScreen: { launchShowDuration: 0 },

		// Passkeys. The WebView cannot run a WebAuthn ceremony for a remote
		// relying party (its origin is capacitor://localhost,
		// not the server's), so lib/webauthn routes native ceremonies through this
		// plugin's platform APIs instead.
		//
		// These pin the native build to ONE instance domain — it cannot be
		// discovered at runtime, because iOS bakes the associated-domains
		// entitlement in at sign time. `autoShim: true` lets the plugin's cap
		// sync/update hook write it into the generated host project.
		//
		// The same domain must serve /.well-known/apple-app-site-association (see
		// deploy/well-known/ and WELL_KNOWN_PATH in docs/configuration.md). Pointing
		// a build at a different instance means changing these two values and
		// re-running `just mobile`.
		CapacitorPasskey: {
			origin: 'https://t.tinyops.ru',
			domains: ['t.tinyops.ru'],
			autoShim: true
		}
	}

	// No server.url -> the bundled SPA shell is served locally by the native
	// layer; the remote API is reached only via fetch/XHR/EventSource.
};

export default config;
