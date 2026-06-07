import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
	PENDING_INVITE_STORAGE_KEY,
	buildCrossInstanceRedirect,
	clearPendingInvite,
	loadPendingInvite,
	normalizeInstanceUrl,
	parseInviteHash,
	parseOwnerHash,
	sameInstance,
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

describe('parseOwnerHash (Federation v1 F2.1, US-2.1 AC2)', () => {
	it('decodes the owner instance URL a cross-instance redirect carries', () => {
		const owner = parseOwnerHash('#invite=theid.thesecret&owner=https%3A%2F%2Falice.example');
		expect(owner).toBe('https://alice.example');
	});

	it('normalizes a scheme-less owner and strips a trailing slash', () => {
		expect(parseOwnerHash('#invite=a.b&owner=alice.example/')).toBe('https://alice.example');
	});

	it('returns null when the fragment carries no owner (link opened on the owner)', () => {
		expect(parseOwnerHash('#invite=theid.thesecret')).toBeNull();
		expect(parseOwnerHash('')).toBeNull();
	});
});

describe('sameInstance', () => {
	it('matches origins differing only by trailing slash or case', () => {
		expect(sameInstance('https://alice.example', 'https://alice.example/')).toBe(true);
		expect(sameInstance('https://Alice.Example', 'https://alice.example')).toBe(true);
	});

	it('distinguishes a different host', () => {
		expect(sameInstance('https://alice.example', 'https://bob.example')).toBe(false);
	});
});

describe('buildCrossInstanceRedirect (Federation v1 F2.1, US-2.1 AC2)', () => {
	const invite = { inviteId: 'theid', secret: 'thesecret' };

	it('targets the joiner /federation/join with the secret AND owner in the FRAGMENT, never the query', () => {
		const url = buildCrossInstanceRedirect('https://my-instance.tld', invite, 'https://alice.example');
		// US-2.1 AC2 / R4: nothing rides in the query string.
		expect(url).not.toContain('?');
		expect(url.startsWith('https://my-instance.tld/federation/join#')).toBe(true);
		// The fragment round-trips back through the parsers.
		const fragment = url.split('#')[1] ?? '';
		expect(parseInviteHash(fragment)).toEqual(invite);
		expect(parseOwnerHash(fragment)).toBe('https://alice.example');
	});

	it('trims a trailing slash on the instance URL so the path is not doubled', () => {
		const url = buildCrossInstanceRedirect('https://my-instance.tld/', invite, 'https://alice.example');
		expect(url.startsWith('https://my-instance.tld/federation/join#')).toBe(true);
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

	it('stashes the invite + owner in sessionStorage and reloads it after login', () => {
		const invite = { inviteId: 'theid', secret: 'thesecret' };
		stashPendingInvite(invite, 'https://alice.example');

		// The raw secret must live only in sessionStorage (cleared on tab close),
		// never localStorage.
		if (typeof localStorage !== 'undefined') {
			expect(localStorage.getItem(PENDING_INVITE_STORAGE_KEY)).toBeNull();
		}
		expect(loadPendingInvite()).toEqual({ invite, owner: 'https://alice.example' });
	});

	it('clears the stash so the invite is consumed exactly once', () => {
		stashPendingInvite({ inviteId: 'theid', secret: 'thesecret' }, 'https://alice.example');
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

	it('returns null when the stash is missing the owner', () => {
		sessionStorage.setItem(
			PENDING_INVITE_STORAGE_KEY,
			JSON.stringify({ inviteId: 'theid', secret: 'thesecret' })
		);
		expect(loadPendingInvite()).toBeNull();
	});
});
