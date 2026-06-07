import type { AuthStore } from './store.svelte';

const AUTH_ROUTES = new Set<string>(['/login', '/setup']);
const PUBLIC_ROUTES = new Set<string>(['/terms-of-service', '/privacy-policy']);
// Route prefixes a guest may reach without being bounced to /login. The
// federation join page is reachable by guests on purpose: when the invite is
// opened on the OWNER instance the visitor has no session there and must still
// see the "open in your instance" prompt (Federation v1 F2.1, US-2.1 AC2). On
// the joiner instance the page itself stashes the invite and redirects to login.
const PUBLIC_PREFIXES = ['/federation/join'];

export function isAuthRoute(pathname: string): boolean {
	return AUTH_ROUTES.has(pathname);
}

export function isPublicRoute(pathname: string): boolean {
	if (PUBLIC_ROUTES.has(pathname)) return true;
	return PUBLIC_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

export type AuthRedirect = '/login' | '/setup' | '/' | null;

// Pure decision function — actual navigation lives in (auth)/(app) layouts so they
// can call goto() with a properly typed route and respect Svelte's resolve() rule.
export function decideAuthRedirect(store: AuthStore, pathname: string): AuthRedirect {
	if (store.status === 'loading') return null;

	if (isPublicRoute(pathname)) return null;

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
