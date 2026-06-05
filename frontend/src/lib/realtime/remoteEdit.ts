// remoteEdit is the F3.4 open-card "updated remotely" affordance (US-3.1 AC3).
//
// Federation-origin changes arrive on the same coarse SSE scopes a normal local
// refresh uses (`tasks`, `projects`, `sections`). The federation inbox publishes
// to the hub WITHOUT an origin (see internal/service/federation/inbox_notify.go),
// so unlike a local mutation — whose echo the originating tab suppresses via
// lib/realtime/origin.ts — a remote peer's edit is NOT echo-suppressed and DOES
// reach an open editor. A blind refresh would silently clobber the user's
// in-flight edit.
//
// createRemoteEditWatcher decides, per incoming invalidation, whether to:
//   - run the normal refresh (the editor has no in-flight edits — US-3.1 AC2), or
//   - hold the change behind a non-destructive notice (the editor is dirty —
//     US-3.1 AC3), exposing Reload (apply the latest) / Keep-editing (dismiss,
//     preserve the draft) actions.
//
// It is deliberately framework-agnostic (no Svelte runes) so the decision logic
// is unit-testable in isolation; the consuming component owns the reactive
// `pending` rendering by reading the getter inside its own `$state`-derived flow
// or by binding to the returned snapshot via an effect.

export interface RemoteEditWatcherOptions {
	// isDirty reports whether the editor currently holds unsaved, in-flight edits
	// that a refresh would overwrite.
	isDirty: () => boolean;
	// refresh applies the latest server state on the BACKGROUND clean-editor path
	// (a remote change arriving while the editor has nothing to lose). The user did
	// NOT initiate it, so failures should be swallowed (no toast) — wire this to the
	// error-swallowing background revalidation.
	refresh: () => void | Promise<void>;
	// reload applies the latest server state on the EXPLICIT user-initiated Reload
	// action. Unlike `refresh`, the user DID ask for this, so its outcome must be
	// surfaced: wire this to the path that runs the loader's onError (mapping
	// 410/not_found → the notFound view, toasting other errors). The watcher awaits
	// it and keeps the notice up if it rejects (see reload() below). Defaults to
	// `refresh` when not provided.
	reload?: () => void | Promise<void>;
	// onChange (optional) is invoked whenever the `pending` flag transitions, so a
	// component can mirror it into reactive state.
	onChange?: (pending: boolean) => void;
}

export interface RemoteEditWatcher {
	// pending is true while a remote change is being held for the user to resolve.
	readonly pending: boolean;
	// onRemoteChange is wired to the SSE invalidation for the editor's scope.
	onRemoteChange(): void;
	// reload applies the latest server state and clears the notice ONLY on success.
	// If the reload rejects (e.g. the peer deleted the task → 410, the project
	// flipped read-only → 403, or the session expired → 401), the notice is kept up
	// so the user does not believe a failed reload succeeded. Returns the awaited
	// reload promise so callers can sequence on it.
	reload(): Promise<void>;
	// keepEditing dismisses the notice WITHOUT refreshing — the draft survives.
	keepEditing(): void;
	// clear resets the notice (e.g. on save or when the editor closes) so it does
	// not linger after the conflict is otherwise resolved.
	clear(): void;
}

export function createRemoteEditWatcher(opts: RemoteEditWatcherOptions): RemoteEditWatcher {
	let pending = false;

	function setPending(next: boolean): void {
		if (pending === next) return;
		pending = next;
		opts.onChange?.(pending);
	}

	return {
		get pending(): boolean {
			return pending;
		},
		onRemoteChange(): void {
			// A clean editor has nothing to lose: behave like the normal F3.2
			// refresh (US-3.1 AC2). A dirty editor must not be clobbered — surface
			// the non-destructive notice instead (US-3.1 AC3). Multiple remote
			// changes coalesce into the single pending notice.
			if (!opts.isDirty()) {
				opts.refresh();
				return;
			}
			setPending(true);
		},
		async reload(): Promise<void> {
			// The user explicitly asked to pull the latest. Surface the OUTCOME: only
			// clear the notice once the reload actually resolves. On rejection (peer
			// delete → 410, read-only → 403, expired session → 401) keep the banner up
			// — the loader's onError has already mapped/toasted the failure, and the
			// stale banner remains the user's signal that they are NOT yet in sync.
			const run = opts.reload ?? opts.refresh;
			try {
				await run();
				setPending(false);
			} catch {
				// Keep pending=true; onError already surfaced the failure.
			}
		},
		keepEditing(): void {
			setPending(false);
		},
		clear(): void {
			setPending(false);
		}
	};
}
