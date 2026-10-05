package ru.tinyops.turboist.core.network.http

import okhttp3.Interceptor
import okhttp3.Request
import okhttp3.Response
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiErrorEnvelope
import ru.tinyops.turboist.core.network.ApiHeaders
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.auth.AccessTokenRefresher
import ru.tinyops.turboist.core.network.auth.AccessTokenSource

/**
 * Carries the session on every request, and repairs it once when it has aged out.
 *
 * Access tokens are short-lived by design, so a token expiring mid-session is
 * routine rather than exceptional: the request is retried with a fresh token and
 * the caller never learns anything happened. An `auth_expired` rejection is the
 * usual trigger. The other one is a request that carried no token at all: a
 * refusal then says the server saw no credentials, not that it rejected any, and
 * a session that exists but has no access token in memory yet is repairable. A
 * plain `auth_invalid` on a request that *did* carry a token means the
 * credentials are wrong rather than stale, and refreshing would only replay the
 * same rejection.
 *
 * **One refresh, not one per request.** A screen that fires five requests at once
 * gets five 401s at once; refreshing five times would rotate the refresh token
 * five times, and the server treats a re-used rotated token as theft and kills
 * the session. So refreshes are serialized, and a request that arrives at the
 * lock to find the token already replaced simply takes the new one.
 *
 * The retry re-sends the original request object, which matters for a mutation:
 * it still carries the same idempotency key, so a write whose response was lost
 * to the expiry cannot execute twice.
 */
class AuthInterceptor(
    private val tokens: AccessTokenSource,
    private val refresher: AccessTokenRefresher,
) : Interceptor {
    private val refreshLock = Any()

    override fun intercept(chain: Interceptor.Chain): Response {
        val original = chain.request()
        // A call that brought its own credentials is left exactly as built.
        if (original.header(ApiHeaders.AUTHORIZATION) != null) return chain.proceed(original)
        // The sign-in endpoints establish a session rather than using one. Leaving
        // them alone is what stops a failing refresh from trying to refresh itself.
        if (UNAUTHENTICATED_PATHS.any { original.url.encodedPath.endsWith(it) }) return chain.proceed(original)

        val sent = tokens.accessToken()
        val first = chain.proceed(original.withBearer(sent))
        if (!first.invitesRepair(sent)) return first

        val refreshed = refreshOnce(sent) ?: return first
        first.close()
        return chain.proceed(original.withBearer(refreshed))
    }

    /**
     * Produces a usable token, refreshing at most once across all threads.
     *
     * The token is re-read inside the lock: whoever waited there while another
     * thread refreshed does not need a refresh of its own, and asking for one
     * would burn the freshly rotated refresh token.
     */
    private fun refreshOnce(sent: String?): String? =
        synchronized(refreshLock) {
            val current = tokens.accessToken()
            if (current != null && current != sent) return current
            refresher.refresh(sent)
        }

    private fun Request.withBearer(token: String?): Request {
        if (token.isNullOrEmpty()) return this
        return newBuilder().header(ApiHeaders.AUTHORIZATION, "Bearer $token").build()
    }

    /**
     * True when this rejection is worth one attempt at a fresh token.
     *
     * Either the server said the token aged out, or the request went out with no
     * token to age: that second shape is what every authenticated call looks like
     * after a launch that could not reach the server, where the stored session
     * survives but no access token has been minted from it yet. Treating that
     * refusal as final would leave the app reading its replica until the process
     * is started again, so it is exactly the case a repair exists for.
     */
    private fun Response.invitesRepair(sent: String?): Boolean {
        if (code != HTTP_UNAUTHORIZED) return false
        return sent.isNullOrEmpty() || isExpiredAccess()
    }

    /**
     * True when the rejection says the access token aged out, as opposed to being
     * wrong. Reading the body here is a peek: the response still has to be
     * returnable to the caller when no refresh happens.
     */
    private fun Response.isExpiredAccess(): Boolean {
        val text =
            try {
                peekBody(MAX_ERROR_BODY_BYTES).string()
            } catch (_: Exception) {
                return false
            }
        val envelope =
            try {
                TurboistJson.decodeFromString(ApiErrorEnvelope.serializer(), text)
            } catch (_: Exception) {
                return false
            }
        return envelope.error?.code == ApiErrorCodes.AUTH_EXPIRED
    }

    private companion object {
        const val HTTP_UNAUTHORIZED = 401

        val UNAUTHENTICATED_PATHS =
            setOf(
                "/api/config",
                "/auth/setup",
                "/auth/login",
                "/auth/login/otp",
                "/auth/refresh",
                // A passkey ceremony is a sign-in too. Beyond the rule above, the
                // retry would resend a `finish` whose challenge the first attempt
                // already spent, turning an honest refusal into a confusing one
                // about an invalid ceremony.
                "/auth/passkey/login/begin",
                "/auth/passkey/login/finish",
            )
    }
}
