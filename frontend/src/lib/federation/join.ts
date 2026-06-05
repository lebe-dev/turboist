// Federation join-flow helpers (Federation v1 F2.1, US-2.1).
//
// The invite secret is a 256-bit credential. It travels ONLY in the URL
// fragment (never the query string, never a logged path) and is stashed ONLY in
// sessionStorage (cleared on tab close, never localStorage) while the visitor
// authenticates. These pure helpers are the single place that discipline lives,
// so the page component and the redirect both go through them.

// ParsedInvite is the decoded contents of an #invite=<id>.<secret> fragment.
export interface ParsedInvite {
	inviteId: string;
	secret: string;
}

// sessionStorage key under which a pending invite is stashed across the login
// round-trip. Exported so tests assert it lands in sessionStorage, not
// localStorage (US-2.1 AC5).
export const PENDING_INVITE_STORAGE_KEY = 'turboist.federation.pendingInvite';

// parseInviteHash decodes an invite from a URL fragment of the form
// `#invite=<id>.<secret>` (US-2.1 AC1). The secret may itself contain dots, so
// only the FIRST dot is treated as the id/secret separator. Returns null when
// the fragment carries no usable invite.
export function parseInviteHash(hash: string): ParsedInvite | null {
	const raw = hash.startsWith('#') ? hash.slice(1) : hash;
	if (raw === '') return null;

	const params = new URLSearchParams(raw);
	const value = params.get('invite');
	if (!value) return null;

	const dot = value.indexOf('.');
	if (dot <= 0) return null;

	const inviteId = value.slice(0, dot);
	const secret = value.slice(dot + 1);
	if (!inviteId || !secret) return null;

	return { inviteId, secret };
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

// buildCrossInstanceRedirect builds the "Open in your instance" link
// (US-2.1 AC2): the joiner pastes the invite onto THEIR own instance's
// /federation/join page, carrying the secret in the fragment so it never
// reaches a server as a query parameter (R4 — fragment-vs-query discipline).
export function buildCrossInstanceRedirect(instanceUrl: string, invite: ParsedInvite): string {
	const origin = instanceUrl.replace(/\/+$/, '');
	return `${origin}/federation/join#invite=${invite.inviteId}.${invite.secret}`;
}

// stashPendingInvite saves an invite in sessionStorage so the join flow can
// resume after an unauthenticated visitor logs in (US-2.1 AC5). It is a no-op
// when sessionStorage is unavailable (SSR).
export function stashPendingInvite(invite: ParsedInvite): void {
	if (typeof sessionStorage === 'undefined') return;
	sessionStorage.setItem(PENDING_INVITE_STORAGE_KEY, JSON.stringify(invite));
}

// loadPendingInvite reads (without clearing) a previously stashed invite.
// Returns null when nothing is stashed or the payload is corrupt.
export function loadPendingInvite(): ParsedInvite | null {
	if (typeof sessionStorage === 'undefined') return null;
	const raw = sessionStorage.getItem(PENDING_INVITE_STORAGE_KEY);
	if (!raw) return null;
	try {
		const parsed = JSON.parse(raw) as Partial<ParsedInvite>;
		if (typeof parsed.inviteId !== 'string' || typeof parsed.secret !== 'string') return null;
		if (!parsed.inviteId || !parsed.secret) return null;
		return { inviteId: parsed.inviteId, secret: parsed.secret };
	} catch {
		return null;
	}
}

// clearPendingInvite removes the stash so an invite is consumed exactly once.
export function clearPendingInvite(): void {
	if (typeof sessionStorage === 'undefined') return;
	sessionStorage.removeItem(PENDING_INVITE_STORAGE_KEY);
}
