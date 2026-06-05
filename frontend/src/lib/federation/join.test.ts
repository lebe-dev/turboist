import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
	PENDING_INVITE_STORAGE_KEY,
	buildCrossInstanceRedirect,
	clearPendingInvite,
	loadPendingInvite,
	normalizeInstanceUrl,
	parseInviteHash,
	stashPendingInvite
} from './join';

describe('parseInviteHash (Federation v1 F2.1, US-2.1 AC1)', () => {
	it('parses #invite=<id>.<secret> into its two parts', () => {
		const parsed = parseInviteHash('#invite=01J0000000000000000000000A.super-secret-256-bit');
		expect(parsed).toEqual({
			inviteId: '01J0000000000000000000000A',
			secret: 'super-secret-256-bit'
		});
	});

	it('tolerates a missing leading # (raw fragment)', () => {
		const parsed = parseInviteHash('invite=abc.def');
		expect(parsed).toEqual({ inviteId: 'abc', secret: 'def' });
	});

	it('keeps only the first dot as the id/secret separator (secret may contain dots)', () => {
		const parsed = parseInviteHash('#invite=theid.part1.part2');
		expect(parsed).toEqual({ inviteId: 'theid', secret: 'part1.part2' });
	});

	it('returns null for an empty hash', () => {
		expect(parseInviteHash('')).toBeNull();
		expect(parseInviteHash('#')).toBeNull();
	});

	it('returns null when the invite param is absent', () => {
		expect(parseInviteHash('#foo=bar')).toBeNull();
	});

	it('returns null when id or secret is missing', () => {
		expect(parseInviteHash('#invite=onlyid')).toBeNull();
		expect(parseInviteHash('#invite=.secretonly')).toBeNull();
		expect(parseInviteHash('#invite=idonly.')).toBeNull();
	});
});

describe('buildCrossInstanceRedirect (Federation v1 F2.1, US-2.1 AC2)', () => {
	const invite = { inviteId: 'theid', secret: 'thesecret' };

	it('targets the joiner instance /federation/join with the secret in the FRAGMENT, never the query', () => {
		const url = buildCrossInstanceRedirect('https://my-instance.tld', invite);
		expect(url).toBe('https://my-instance.tld/federation/join#invite=theid.thesecret');
		// US-2.1 AC2 / R4: the secret must never leak into the query string.
		expect(url).not.toContain('?');
		const [, fragment] = url.split('#');
		expect(fragment).toBe('invite=theid.thesecret');
	});

	it('trims a trailing slash on the instance URL so the path is not doubled', () => {
		const url = buildCrossInstanceRedirect('https://my-instance.tld/', invite);
		expect(url).toBe('https://my-instance.tld/federation/join#invite=theid.thesecret');
	});
});

describe('normalizeInstanceUrl', () => {
	it('prepends https:// when no scheme is present', () => {
		expect(normalizeInstanceUrl('alice.example')).toBe('https://alice.example');
	});

	it('preserves an explicit scheme and strips a trailing slash', () => {
		expect(normalizeInstanceUrl('http://localhost:3000/')).toBe('http://localhost:3000');
	});

	it('trims surrounding whitespace', () => {
		expect(normalizeInstanceUrl('  https://bob.example  ')).toBe('https://bob.example');
	});

	it('returns null for an empty value', () => {
		expect(normalizeInstanceUrl('')).toBeNull();
		expect(normalizeInstanceUrl('   ')).toBeNull();
	});
});

describe('pending-invite session stash (Federation v1 F2.1, US-2.1 AC5)', () => {
	beforeEach(() => {
		sessionStorage.clear();
	});
	afterEach(() => {
		sessionStorage.clear();
	});

	it('stashes the invite in sessionStorage and reloads it after login', () => {
		const invite = { inviteId: 'theid', secret: 'thesecret' };
		stashPendingInvite(invite);

		// The raw secret must live only in sessionStorage (cleared on tab close),
		// never localStorage.
		if (typeof localStorage !== 'undefined') {
			expect(localStorage.getItem(PENDING_INVITE_STORAGE_KEY)).toBeNull();
		}
		expect(sessionStorage.getItem(PENDING_INVITE_STORAGE_KEY)).toBe(JSON.stringify(invite));
		expect(loadPendingInvite()).toEqual(invite);
	});

	it('clears the stash so the invite is consumed exactly once', () => {
		stashPendingInvite({ inviteId: 'theid', secret: 'thesecret' });
		clearPendingInvite();
		expect(loadPendingInvite()).toBeNull();
	});

	it('returns null when nothing is stashed', () => {
		expect(loadPendingInvite()).toBeNull();
	});

	it('returns null for a corrupt stash payload', () => {
		sessionStorage.setItem(PENDING_INVITE_STORAGE_KEY, 'not-json');
		expect(loadPendingInvite()).toBeNull();
	});
});
