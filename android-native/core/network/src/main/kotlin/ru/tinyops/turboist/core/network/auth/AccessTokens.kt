package ru.tinyops.turboist.core.network.auth

/**
 * Where the HTTP layer reads the short-lived access token from.
 *
 * The token itself is deliberately not owned here: it lives in memory in the
 * session layer, next to the rotating refresh token that produces it, and this
 * module only borrows it once per request. Returning `null` means "not signed
 * in", and the request goes out unauthenticated — which is correct for the login
 * and setup calls and produces an honest 401 everywhere else.
 */
fun interface AccessTokenSource {
    fun accessToken(): String?

    companion object {
        /** No session at all. What an unauthenticated client — the connect screen — uses. */
        val None: AccessTokenSource = AccessTokenSource { null }
    }
}

/**
 * Exchanges an aged-out access token for a fresh one.
 *
 * The HTTP layer calls this at most once per rejected request and never more
 * than once at a time, no matter how many requests were rejected together. The
 * implementation owns the rotating refresh token, its storage and the decision
 * to give up and sign the user out; all this layer needs back is the new access
 * token, or `null` when there is not going to be one.
 *
 * It is called from the calling thread while that request is parked, so it may
 * block, and it must not go through the same authenticated path it is repairing.
 */
fun interface AccessTokenRefresher {
    /**
     * @param expiredToken the token the rejected request carried, so an
     *   implementation can tell a genuinely stale token from a race it already won.
     * @return the replacement token, or `null` to let the original rejection stand.
     */
    fun refresh(expiredToken: String?): String?

    companion object {
        /** Never recovers. What a client with no session — or a test — uses. */
        val None: AccessTokenRefresher = AccessTokenRefresher { null }
    }
}
