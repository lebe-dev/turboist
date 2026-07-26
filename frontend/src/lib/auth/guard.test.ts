import { describe, expect, it } from 'vitest';
import { decideAuthRedirect, isAuthRoute, isPublicRoute } from './guard';
import type { AuthStore } from './store.svelte';

// decideAuthRedirect only reads `status`, `setupRequired` and `offlineSession`,
// so a structural stub is enough to exercise the redirect matrix in isolation.
function mkStore(partial: Partial<AuthStore>): AuthStore {
	return {
		status: 'guest',
		setupRequired: false,
		offlineSession: false,
		...partial
	} as AuthStore;
}

describe('decideAuthRedirect', () => {
	it('never redirects while loading', () => {
		expect(decideAuthRedirect(mkStore({ status: 'loading' }), '/today')).toBeNull();
	});

	it('leaves public routes alone', () => {
		expect(decideAuthRedirect(mkStore({ status: 'guest' }), '/terms-of-service')).toBeNull();
		expect(decideAuthRedirect(mkStore({ status: 'guest' }), '/privacy-policy')).toBeNull();
	});

	it('guest on an app route → /login', () => {
		expect(decideAuthRedirect(mkStore({ status: 'guest' }), '/today')).toBe('/login');
	});

	it('guest already on /login → stay', () => {
		expect(decideAuthRedirect(mkStore({ status: 'guest' }), '/login')).toBeNull();
	});

	it('setupRequired forces /setup', () => {
		expect(decideAuthRedirect(mkStore({ setupRequired: true }), '/today')).toBe('/setup');
		expect(decideAuthRedirect(mkStore({ setupRequired: true }), '/setup')).toBeNull();
	});

	it('authenticated on an auth route → /', () => {
		expect(decideAuthRedirect(mkStore({ status: 'authenticated' }), '/login')).toBe('/');
	});

	it('authenticated on an app route → stay', () => {
		expect(decideAuthRedirect(mkStore({ status: 'authenticated' }), '/today')).toBeNull();
	});

	// §4.9 — offline boot: the server could not be reached, so we could not verify
	// the session. Never bounce to /login; render the current route from cache.
	it('offlineSession on an app route → stay (no /login)', () => {
		const store = mkStore({ status: 'authenticated', offlineSession: true });
		expect(decideAuthRedirect(store, '/today')).toBeNull();
	});

	it('offlineSession even with guest status must NOT redirect to /login', () => {
		const store = mkStore({ status: 'guest', offlineSession: true });
		expect(decideAuthRedirect(store, '/today')).toBeNull();
	});

	it('offlineSession on /login → / (surface the cached workspace)', () => {
		const store = mkStore({ status: 'authenticated', offlineSession: true });
		expect(decideAuthRedirect(store, '/login')).toBe('/');
	});
});

describe('route predicates', () => {
	it('isAuthRoute', () => {
		expect(isAuthRoute('/login')).toBe(true);
		expect(isAuthRoute('/setup')).toBe(true);
		expect(isAuthRoute('/today')).toBe(false);
	});

	it('isPublicRoute', () => {
		expect(isPublicRoute('/terms-of-service')).toBe(true);
		expect(isPublicRoute('/today')).toBe(false);
	});
});
