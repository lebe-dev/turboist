import { describe, expect, it } from 'vitest';
import { decideAuthRedirect, isPublicRoute } from './guard';
import type { AuthStore } from './store.svelte';

// Minimal AuthStore stand-in: decideAuthRedirect only reads status + setupRequired.
function store(status: AuthStore['status'], setupRequired = false): AuthStore {
	return { status, setupRequired } as AuthStore;
}

describe('isPublicRoute', () => {
	it('treats the federation join page (and its subpaths) as public', () => {
		expect(isPublicRoute('/federation/join')).toBe(true);
		expect(isPublicRoute('/federation/join/accept')).toBe(true);
	});

	it('does not treat other federation routes as public', () => {
		expect(isPublicRoute('/federation/events')).toBe(false);
		expect(isPublicRoute('/federation/joiner')).toBe(false);
	});
});

describe('decideAuthRedirect — federation join (Federation v1 F2.1, US-2.1 AC2)', () => {
	it('does NOT bounce a guest off /federation/join to /login', () => {
		// The owner-instance visitor has no session there but must still see the
		// "open in your instance" prompt — the global guard must not redirect.
		expect(decideAuthRedirect(store('guest'), '/federation/join')).toBeNull();
	});

	it('still bounces a guest off a protected route to /login', () => {
		expect(decideAuthRedirect(store('guest'), '/project/2')).toBe('/login');
	});

	it('does not interfere with an authenticated visitor on the join page', () => {
		expect(decideAuthRedirect(store('authenticated'), '/federation/join')).toBeNull();
	});
});
