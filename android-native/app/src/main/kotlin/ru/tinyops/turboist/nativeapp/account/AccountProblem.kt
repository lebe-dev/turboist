package ru.tinyops.turboist.nativeapp.account

import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException

/**
 * Why an administrative call did not do what was asked.
 *
 * Four answers, because they lead to four different things for the user to do,
 * and collapsing them would leave the screen saying "something went wrong" to a
 * person who simply mistyped a six-digit code.
 */
enum class AccountProblem {
    /**
     * Nothing reached the server.
     *
     * These screens have no stored copy to fall back on — deliberately, because
     * a stale answer about which devices can sign in is worse than no answer —
     * so this is the one failure they have to state plainly rather than paper
     * over.
     */
    OFFLINE,

    /**
     * The server does not offer this at all: the routes are simply not there.
     *
     * The second factor is optional in a deployment, and an instance configured
     * without one never registers its endpoints. That is an honest "not
     * available here" rather than a fault, and the screen says so instead of
     * inviting the user to keep trying.
     */
    UNAVAILABLE,

    /** The code was wrong or had already expired. Typing another one is the fix. */
    INVALID_CODE,

    /** The server understood and refused. Retrying the same request changes nothing. */
    REFUSED,
}

/**
 * Reads a failure as the answer it is.
 *
 * A refusal on a path that exists is [AccountProblem.REFUSED] whatever its
 * status, including a 404: a session id that names nothing means the session is
 * already gone, not that the server lacks the feature. Only the caller knows
 * which of the two a missing route would mean, so that reading is left to
 * [endpointIsAbsent] rather than guessed at here.
 */
fun accountProblemOf(error: Throwable): AccountProblem =
    when {
        error is ApiException.Network -> AccountProblem.OFFLINE
        error is ApiException && error.code == ApiErrorCodes.TOTP_INVALID_CODE -> AccountProblem.INVALID_CODE
        else -> AccountProblem.REFUSED
    }

/**
 * True when the server answered as though the route does not exist.
 *
 * Asked only where a deployment is genuinely allowed to leave a feature out. The
 * absent route carries no error code of its own — it is the router's own refusal
 * rather than the application's — so the status is what identifies it.
 */
fun endpointIsAbsent(error: Throwable): Boolean {
    if (error !is ApiException) return false
    return error.status == HTTP_NOT_FOUND || error.status == HTTP_METHOD_NOT_ALLOWED
}

private const val HTTP_NOT_FOUND = 404
private const val HTTP_METHOD_NOT_ALLOWED = 405
