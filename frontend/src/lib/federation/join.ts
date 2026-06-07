// Federation join-flow helpers (Federation v1 F2.1, US-2.1).
//
// The invite secret is a 256-bit credential. It travels ONLY in the URL
// fragment (never the query string, never a logged path) and is stashed ONLY in
// sessionStorage (cleared on tab close, never localStorage) while the visitor
// authenticates. These pure helpers are the single place that discipline lives,
// so the page component and the redirect both go through them.

// ParsedInvite is the decoded credential of an #invite=<id>.<secret> fragment.
export interface ParsedInvite {
	inviteId: string;
	secret: string;
}

// PendingJoin is the full join context: the credential plus the owner instance
// URL the invite is FOR. The owner is the link's host, so it must travel with
// the invite when the visitor retargets to their own instance (cross-instance
// redirect) and across the login round-trip — otherwise the joiner instance
// loses track of who the owner is and would handshake itself.
export interface PendingJoin {
	invite: ParsedInvite;
	owner: string;
}

// sessionStorage key under which a pending join is stashed across the login
// round-trip. Exported so tests assert it lands in sessionStorage, not
// localStorage (US-2.1 AC5).
export const PENDING_INVITE_STORAGE_KEY = 'turboist.federation.pendingInvite';

// parseInviteHash decodes the invite credential from a URL fragment of the form
// `#invite=<id>.<secret>` (US-2.1 AC1). The secret may itself contain dots, so
// only the FIRST dot is treated as the id/secret separator. Returns null when
// the fragment carries no usable invite.
export function parseInviteHash(hash: string): ParsedInvite | null {
	const params = fragmentParams(hash);
	if (!params) return null;

	const value = params.get('invite');
	if (!value) return null;

	const dot = value.indexOf('.');
	if (dot <= 0) return null;

	const inviteId = value.slice(0, dot);
	const secret = value.slice(dot + 1);
	if (!inviteId || !secret) return null;

	return { inviteId, secret };
}

// parseOwnerHash decodes the optional `owner=<instance-url>` fragment param a
// cross-instance redirect carries (US-2.1 AC2). Returns the normalized owner
// origin, or null when the fragment carries no owner (a freshly issued invite
// link opened directly on the owner — the caller falls back to the page origin).
export function parseOwnerHash(hash: string): string | null {
	const params = fragmentParams(hash);
	if (!params) return null;

	const owner = params.get('owner');
	if (!owner) return null;
	return normalizeInstanceUrl(owner);
}

// fragmentParams parses the `#a=b&c=d` portion of a URL fragment into a
// URLSearchParams, or null when the fragment is empty.
function fragmentParams(hash: string): URLSearchParams | null {
	const raw = hash.startsWith('#') ? hash.slice(1) : hash;
	if (raw === '') return null;
	return new URLSearchParams(raw);
}

// normalizeInstanceUrl coerces user-typed instance input into an absolute URL
// origin: it prepends https:// when no scheme is present and strips a trailing
// slash so paths are not doubled. Returns null for blank input.
export function normalizeInstanceUrl(input: string): string | null {
	const trimmed = input.trim();
	if (trimmed === '') return null;

	const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
	return withScheme.replace(/\/+$/, '');
}

// sameInstance reports whether two instance URLs name the same origin, tolerant
// of trailing-slash and case differences. Used to decide whether the page is
// being served BY the invite's owner (no join here — retarget to your own
// instance) or by the joiner (proceed with preview/join).
export function sameInstance(a: string, b: string): boolean {
	const na = normalizeInstanceUrl(a);
	const nb = normalizeInstanceUrl(b);
	if (na === null || nb === null) return false;
	return na.toLowerCase() === nb.toLowerCase();
}

// buildCrossInstanceRedirect builds the "Open in your instance" link
// (US-2.1 AC2): the joiner opens the invite on THEIR own instance's
// /federation/join page. The secret rides in the fragment so it never reaches a
// server as a query parameter (R4 — fragment-vs-query discipline), and the owner
// URL rides alongside it so the joiner instance knows which owner to handshake.
export function buildCrossInstanceRedirect(
	instanceUrl: string,
	invite: ParsedInvite,
	ownerUrl: string
): string {
	const origin = instanceUrl.replace(/\/+$/, '');
	const params = new URLSearchParams();
	params.set('invite', `${invite.inviteId}.${invite.secret}`);
	params.set('owner', ownerUrl);
	return `${origin}/federation/join#${params.toString()}`;
}

// stashPendingInvite saves a join context in sessionStorage so the flow can
// resume after an unauthenticated visitor logs in (US-2.1 AC5). It is a no-op
// when sessionStorage is unavailable (SSR).
export function stashPendingInvite(invite: ParsedInvite, owner: string): void {
	if (typeof sessionStorage === 'undefined') return;
	const payload = { inviteId: invite.inviteId, secret: invite.secret, owner };
	sessionStorage.setItem(PENDING_INVITE_STORAGE_KEY, JSON.stringify(payload));
}

// loadPendingInvite reads (without clearing) a previously stashed join context.
// Returns null when nothing is stashed or the payload is corrupt.
export function loadPendingInvite(): PendingJoin | null {
	if (typeof sessionStorage === 'undefined') return null;
	const raw = sessionStorage.getItem(PENDING_INVITE_STORAGE_KEY);
	if (!raw) return null;
	try {
		const parsed = JSON.parse(raw) as Partial<ParsedInvite & { owner: string }>;
		if (typeof parsed.inviteId !== 'string' || typeof parsed.secret !== 'string') return null;
		if (typeof parsed.owner !== 'string' || !parsed.owner) return null;
		if (!parsed.inviteId || !parsed.secret) return null;
		return { invite: { inviteId: parsed.inviteId, secret: parsed.secret }, owner: parsed.owner };
	} catch {
		return null;
	}
}

// clearPendingInvite removes the stash so an invite is consumed exactly once.
export function clearPendingInvite(): void {
	if (typeof sessionStorage === 'undefined') return;
	sessionStorage.removeItem(PENDING_INVITE_STORAGE_KEY);
}
