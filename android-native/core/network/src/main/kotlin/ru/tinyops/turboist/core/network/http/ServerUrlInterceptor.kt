package ru.tinyops.turboist.core.network.http

import okhttp3.Interceptor
import okhttp3.Response
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.ServerUrl

/**
 * Points every request at the server the user connected to.
 *
 * Retrofit fixes its base URL when the client is built, but this app learns its
 * server address at runtime and can be pointed at a different one without a
 * restart. So the endpoint interfaces are built against a placeholder and this
 * interceptor rewrites scheme, host, port and path prefix on the way out — which
 * also means a change of address takes effect on the very next request instead of
 * requiring the whole HTTP stack to be rebuilt.
 *
 * A request made before an address was ever configured fails as a network error
 * rather than reaching the placeholder host.
 */
class ServerUrlInterceptor(
    private val serverUrl: ServerUrl,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val target = serverUrl.value ?: throw ApiException.Network("no server address is configured")
        val request = chain.request()
        val rewritten =
            request.url.newBuilder()
                .scheme(target.scheme)
                .host(target.host)
                .port(target.port)
                // The address may point into a sub-path (a reverse proxy mounting
                // the app under /turboist), so the endpoint path is appended to it
                // rather than replacing it.
                .encodedPath(target.encodedPath.trimEnd('/') + request.url.encodedPath)
                .build()
        return chain.proceed(request.newBuilder().url(rewritten).build())
    }

    companion object {
        /**
         * The address the endpoint interfaces are compiled against. It is never
         * contacted: every request is rewritten before it leaves. `.invalid` is
         * reserved by the DNS standard precisely so it can never resolve, so a
         * rewrite that failed to happen fails loudly instead of leaking a request
         * to a real host.
         */
        const val PLACEHOLDER_BASE_URL: String = "http://server.invalid/"
    }
}
