import type { AuthStore } from './store.svelte';

const AUTH_ROUTES = new Set<string>(['/login', '/setup']);

export function isAuthRoute(pathname: string): boolean {
	return AUTH_ROUTES.has(pathname);
}

export type AuthRedirect = '/login' | '/setup' | '/' | null;

// Pure decision function — actual navigation lives in (auth)/(app) layouts so they
// can call goto() with a properly typed route and respect Svelte's resolve() rule.
export function decideAuthRedirect(store: AuthStore, pathname: string): AuthRedirect {
	if (store.status === 'loading') return null;

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
