// Logout orchestration (FEATURE-OFFLINE-ARCH.md §4.9).
//
// Logging out discards the offline read-through cache and outbox. When the
// outbox still holds unsent changes, those would be lost — so the flow asks for
// confirmation FIRST and only clears the IndexedDB once the user agrees. If the
// user cancels, nothing is touched: the session and the queued work both stay
// intact so a later reconnect can still replay them.

export interface LogoutFlowDeps {
	/** Current outbox size (statusStore.pendingOps). */
	pendingOps: () => number;
	/**
	 * Current quarantined-op count (statusStore.failedOps). Optional so existing
	 * callers keep working; both counts must be gone before we wipe silently.
	 * Defaults to 0.
	 */
	failedOps?: () => number;
	/**
	 * Confirm discarding `count` unsent changes. Resolve true to proceed, false
	 * to abort. Only called when there is at least one unsent (pending or failed)
	 * change.
	 */
	confirmDiscard: (count: number) => Promise<boolean>;
	/** Server logout + clear in-memory / native-secure-storage session. */
	logout: () => Promise<void>;
	/** Wipe the offline cache + outbox + failedOps (only reached after confirmation). */
	clearOffline: () => Promise<void>;
}

/**
 * Run the logout flow. Returns true when logout completed, false when the user
 * cancelled at the unsent-changes confirmation (in which case neither the
 * session nor the offline database is touched).
 */
export async function runLogout(deps: LogoutFlowDeps): Promise<boolean> {
	// Both queued and quarantined changes are lost by the wipe — gate on their sum
	// so failedOps are never discarded without asking (§4.7.3, §4.9).
	const unsent = deps.pendingOps() + (deps.failedOps?.() ?? 0);
	if (unsent > 0) {
		const confirmed = await deps.confirmDiscard(unsent);
		if (!confirmed) return false;
	}
	await deps.logout();
	await deps.clearOffline();
	return true;
}
