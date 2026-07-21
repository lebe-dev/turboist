/**
 * Pure guard for the deploy-version forced reload (FEATURE-OFFLINE-ARCH.md
 * §5.3). Kept dependency-free — no `$app/*` runes, no `statusStore` import — so
 * it can be unit-tested in jsdom, mirroring `sw/routing.ts`.
 */

export interface ForceReloadInput {
	/** Native (Capacitor) build: assets are local and `version.json` never flips. */
	native: boolean;
	/** A newer deploy has been detected (`updated.current` from `$app/state`). */
	updated: boolean;
	/** The navigation already unloads the page (`nav.willUnload`) — nothing to force. */
	willUnload: boolean;
	/** The navigation has a resolvable target URL to reload into (`nav.to?.url`). */
	hasTarget: boolean;
	/**
	 * The offline heuristic reports we can reach the server (`statusStore.online`).
	 * Reloading offline — before the service worker is active — would white-screen,
	 * and once the SW is active it is pointless, so the forced reload is gated on
	 * being online (§5.3).
	 */
	online: boolean;
}

/**
 * Whether `beforeNavigate` should cancel the SPA navigation and hard-reload into
 * the fresh bundle: web-only, only when a new deploy is live, and — per §5.3 —
 * only while online.
 */
export function shouldForceReload(input: ForceReloadInput): boolean {
	return (
		!input.native && input.updated && !input.willUnload && input.hasTarget && input.online
	);
}
