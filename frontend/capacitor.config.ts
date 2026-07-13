import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
	appId: 'com.itkey.turboist',
	appName: 'Turboist',
	// SvelteKit adapter-static output (relative to this file). `yarn build` must
	// run before `cap add`/`cap sync`.
	webDir: 'build',

	// Schemes are LEFT AT DEFAULTS on purpose:
	//   iOS     iosScheme 'capacitor' -> origin capacitor://localhost
	//   Android androidScheme 'https' -> origin https://localhost
	// Both are secure contexts, so crypto.randomUUID() (lib/realtime/origin.ts)
	// works. Do NOT set androidScheme:'http' — http://localhost is not a secure
	// context and would break it.

	plugins: {
		// Patch window.fetch + XMLHttpRequest to the native HTTP stack. Every REST
		// call then goes through the native bridge and bypasses WebView CORS, so
		// the backend needs CORS ONLY for the SSE EventSource (which CapacitorHttp
		// does NOT patch — see internal/httpapi/handlers/events.go).
		CapacitorHttp: { enabled: true },

		// CapacitorCookies is intentionally left disabled: native auth uses body
		// refresh tokens persisted in the Keychain/Keystore, not the web HttpOnly
		// cookie (the backend sets that cookie only for clientKind 'web').

		SplashScreen: { launchShowDuration: 0 }
	}

	// No server.url -> the bundled SPA shell is served locally by the native
	// layer; the remote API is reached only via fetch/XHR/EventSource.
};

export default config;
