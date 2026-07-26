import type { AuthStore } from './store.svelte';

const AUTH_ROUTES = new Set<string>(['/login', '/setup']);
const PUBLIC_ROUTES = new Set<string>(['/terms-of-service', '/privacy-policy']);

export function isAuthRoute(pathname: string): boolean {
	return AUTH_ROUTES.has(pathname);
}

export function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.has(pathname);
}

export type AuthRedirect = '/login' | '/setup' | '/' | null;

// Pure decision function — actual navigation lives in (auth)/(app) layouts so they
// can call goto() with a properly typed route and respect Svelte's resolve() rule.
export function decideAuthRedirect(store: AuthStore, pathname: string): AuthRedirect {
	if (store.status === 'loading') return null;

	if (isPublicRoute(pathname)) return null;

	// Offline boot (§4.9): the server was unreachable at startup, so the session
	// could not be validated. Never bounce to /login — stay on the current route
	// and render from the read-through cache. A real rejection (401/403) clears
	// offlineSession and flips status to 'guest', which falls through below.
	if (store.offlineSession) {
		return isAuthRoute(pathname) ? '/' : null;
	}

	if (store.setupRequired) {
		return pathname === '/setup' ? null : '/setup';
	}

	if (store.status === 'guest') {
		return isAuthRoute(pathname) ? null : '/login';
	}

	if (store.status === 'authenticated' && isAuthRoute(pathname)) {
		return '/';
	}
	return null;
}
