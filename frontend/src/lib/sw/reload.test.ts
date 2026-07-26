import { describe, expect, it } from 'vitest';
import { shouldForceReload } from './reload';

// A newer deploy is live, on web, with a real target, online — the one case
// that forces a hard reload. Each test flips a single field off `base`.
const base = {
	native: false,
	updated: true,
	willUnload: false,
	hasTarget: true,
	online: true
};

describe('shouldForceReload', () => {
	it('forces a reload when a new deploy is live and we are online (web)', () => {
		expect(shouldForceReload(base)).toBe(true);
	});

	it('never forces a reload on native — the bundle is local (§5.3)', () => {
		expect(shouldForceReload({ ...base, native: true })).toBe(false);
	});

	it('does nothing until a newer deploy has been detected', () => {
		expect(shouldForceReload({ ...base, updated: false })).toBe(false);
	});

	it('skips a navigation that is already a full page unload', () => {
		expect(shouldForceReload({ ...base, willUnload: true })).toBe(false);
	});

	it('skips a navigation with no resolvable target URL', () => {
		expect(shouldForceReload({ ...base, hasTarget: false })).toBe(false);
	});

	it('does NOT reload while offline — a pre-SW reload would white-screen (§5.3)', () => {
		expect(shouldForceReload({ ...base, online: false })).toBe(false);
	});
});
