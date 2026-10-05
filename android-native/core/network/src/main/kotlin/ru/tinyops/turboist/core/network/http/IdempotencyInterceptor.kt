package ru.tinyops.turboist.core.network.http

import okhttp3.Interceptor
import okhttp3.Response
import ru.tinyops.turboist.core.network.ApiHeaders
import java.util.UUID

/**
 * Makes every mutation safe to retry.
 *
 * A write whose response never arrived is the hard case in an offline-first
 * client: the change may or may not have been committed, and asking again risks
 * a second task, a second recurrence advance, a second pin. An idempotency key
 * removes the ambiguity — the server executes the first request carrying a given
 * key and replays its stored response for any repeat.
 *
 * A caller that owns a key — a queued write, which must replay under the key it
 * was stored with, not a new one — sets the header itself and this interceptor
 * leaves it alone. Everything else gets a fresh key here, which is also why this
 * interceptor sits *outside* the one that retries after a token refresh: the
 * retry reuses the request it was handed, key included.
 *
 * Only the versioned API honours the header, so nothing else is stamped.
 */
class IdempotencyInterceptor(
    private val newKey: () -> String = { UUID.randomUUID().toString() },
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val alreadyKeyed = request.header(ApiHeaders.IDEMPOTENCY_KEY) != null
        if (alreadyKeyed || !request.method.uppercase().let(MUTATING_METHODS::contains)) {
            return chain.proceed(request)
        }
        if (!request.url.encodedPath.contains(VERSIONED_API_PREFIX)) return chain.proceed(request)
        return chain.proceed(request.newBuilder().header(ApiHeaders.IDEMPOTENCY_KEY, newKey()).build())
    }
}

/**
 * True when the server answered by replaying a response it had stored under this
 * request's idempotency key, rather than by executing the request again.
 *
 * A queued write that comes back replayed already landed on an earlier attempt —
 * useful to know when deciding whether a retry actually changed anything.
 */
val Response.isIdempotentReplay: Boolean
    get() = header(ApiHeaders.IDEMPOTENT_REPLAY)?.equals("true", ignoreCase = true) == true

/** The same marker on a Retrofit response, which is what an endpoint returning `Response<T>` hands back. */
val retrofit2.Response<*>.isIdempotentReplay: Boolean
    get() = headers()[ApiHeaders.IDEMPOTENT_REPLAY]?.equals("true", ignoreCase = true) == true
