package ru.tinyops.turboist.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.longOrNull
import java.io.IOException

/** The `{error: {code, message, details}}` body every failed API call carries. */
@Serializable
data class ApiErrorBody(
    @SerialName("code") val code: String = "",
    @SerialName("message") val message: String = "",
    @SerialName("details") val details: JsonObject? = null,
)

@Serializable
internal data class ApiErrorEnvelope(
    @SerialName("error") val error: ApiErrorBody? = null,
)

/**
 * Everything the API layer throws.
 *
 * It extends [IOException] for two reasons. Retrofit propagates an IOException
 * raised inside an interceptor to the caller untouched, which is where this
 * mapping happens; and a caller that only wants to know "the call did not
 * succeed" can catch one type instead of two families.
 *
 * The four subclass families answer the only question a caller has to decide on
 * before looking at anything else — *what do I do now?*
 *
 * - [Network]: nothing reached the server. Retry later; keep local state.
 * - [Auth]: the credentials are the problem. Refresh, or send the user to login.
 * - [Business]: the server understood the request and refused it. Retrying the
 *   identical request will fail identically, so a queued write is quarantined
 *   rather than retried.
 * - [Server]: the server broke. The request itself may well be fine, so a bounded
 *   retry is worth it.
 *
 * [code] and [details] survive the mapping intact, because the interesting part
 * of a refusal lives there: which tasks block this one, which epoch the server is
 * on now.
 */
sealed class ApiException(
    /** The server's error code, or one of the two locally-raised codes. */
    val code: String,
    /** The HTTP status, or `0` when the request never got a response. */
    val status: Int,
    /** The `details` object verbatim, `null` when the server sent none. */
    val details: JsonObject?,
    message: String,
    cause: Throwable? = null,
) : IOException(message, cause) {
    /** Nothing reached the server: no connection, a refused one, or a timeout. */
    class Network(
        message: String,
        cause: Throwable? = null,
    ) : ApiException(ApiErrorCodes.NETWORK_ERROR, 0, null, message, cause)

    /**
     * The request was rejected on credentials: an expired or invalid access token,
     * a missing scope, a wrong second factor.
     *
     * @property isExpiredAccess true for the one case that is recoverable without
     *   the user: the access token aged out and a refresh can replace it.
     */
    class Auth(
        code: String,
        status: Int,
        details: JsonObject?,
        message: String,
    ) : ApiException(code, status, details, message) {
        val isExpiredAccess: Boolean
            get() = code == ApiErrorCodes.AUTH_EXPIRED
    }

    /** The instance has no account yet: every call is refused until setup runs. */
    class SetupRequired(
        message: String,
    ) : ApiException(ApiErrorCodes.SETUP_REQUIRED, 503, null, message)

    /** Too many attempts from this address. The server throttles the login endpoints per IP. */
    class RateLimited(
        message: String,
    ) : ApiException(ApiErrorCodes.AUTH_RATE_LIMITED, 429, null, message)

    /**
     * The server understood the request and refused it on its own rules — a
     * capacity limit, an invalid placement, a task that something still blocks.
     * The same request will be refused again, so it must not be retried blindly.
     */
    class Business(
        code: String,
        status: Int,
        details: JsonObject?,
        message: String,
    ) : ApiException(code, status, details, message)

    /** The server failed to answer properly. The request may be fine; a bounded retry is reasonable. */
    class Server(
        code: String,
        status: Int,
        details: JsonObject?,
        message: String,
    ) : ApiException(code, status, details, message)

    /**
     * The change history this replica was resuming against was replaced — a
     * restore rewrote the data — so its cursor means nothing any more. The remedy
     * is a fresh snapshot, stamped with [currentEpoch].
     */
    class SyncEpochMismatch(
        val currentEpoch: Long,
        details: JsonObject?,
        message: String,
    ) : ApiException(ApiErrorCodes.SYNC_EPOCH_MISMATCH, 409, details, message)

    /**
     * The changes this replica still needs were pruned from the log, so no
     * sequence of delta pulls can catch it up. Same remedy as an epoch mismatch: a
     * fresh snapshot.
     *
     * @property oldestRetained the oldest change the server still holds, which is
     *   what makes the gap visible in a log line.
     */
    class SyncCursorExpired(
        val epoch: Long,
        val oldestRetained: Long,
        details: JsonObject?,
        message: String,
    ) : ApiException(ApiErrorCodes.SYNC_CURSOR_EXPIRED, 410, details, message)

    /**
     * The ids of the still-open tasks that block the task this call tried to
     * complete. Empty for every other failure.
     */
    val blockerIds: List<Long>
        get() = if (code == ApiErrorCodes.TASK_BLOCKED) details.longList("blockerIds") else emptyList()

    companion object {
        /**
         * Maps one failed response onto its typed exception.
         *
         * The specific sync and blocked-task subclasses come first: they are the
         * ones the sync engine branches on, and burying their numbers in a generic
         * [Business] would force every call site to re-parse `details`.
         */
        fun from(
            status: Int,
            body: ApiErrorBody?,
        ): ApiException {
            val code = body?.code?.takeIf { it.isNotEmpty() } ?: ApiErrorCodes.UNKNOWN_ERROR
            val message = body?.message?.takeIf { it.isNotEmpty() } ?: "HTTP $status"
            val details = body?.details
            return when {
                code == ApiErrorCodes.SYNC_EPOCH_MISMATCH ->
                    SyncEpochMismatch(details.long("epoch") ?: 0L, details, message)

                code == ApiErrorCodes.SYNC_CURSOR_EXPIRED ->
                    SyncCursorExpired(
                        details.long("epoch") ?: 0L,
                        details.long("oldestRetained") ?: 0L,
                        details,
                        message,
                    )

                code == ApiErrorCodes.SETUP_REQUIRED -> SetupRequired(message)
                code == ApiErrorCodes.AUTH_RATE_LIMITED || status == 429 -> RateLimited(message)
                status == 401 || status == 403 -> Auth(code, status, details, message)
                status >= 500 -> Server(code, status, details, message)
                else -> Business(code, status, details, message)
            }
        }
    }
}

/** Reads one numeric field out of an error's `details`, tolerating an absent or non-numeric one. */
private fun JsonObject?.long(name: String): Long? = (this?.get(name) as? JsonPrimitive)?.longOrNull

/** Reads one numeric array out of an error's `details`, skipping entries that are not numbers. */
private fun JsonObject?.longList(name: String): List<Long> {
    val array = this?.get(name) as? JsonArray ?: return emptyList()
    return array.mapNotNull { (it as? JsonPrimitive)?.longOrNull }
}
