package ru.tinyops.turboist.core.network.api

import retrofit2.Response

/**
 * The body of a call that succeeded.
 *
 * Mutations are declared as `Response<T>` so the caller draining queued writes
 * can read the response headers — specifically the marker saying the server
 * replayed a stored answer instead of executing the write again. Every failure
 * has already been turned into a typed exception before a `Response` is handed
 * back, so a body that is missing here is a server that answered a 2xx with
 * nothing, which no endpoint is supposed to do.
 */
fun <T : Any> Response<T>.requireBody(): T =
    body() ?: throw IllegalStateException("the server answered ${code()} with an empty body")
