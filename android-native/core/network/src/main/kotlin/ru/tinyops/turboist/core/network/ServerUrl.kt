package ru.tinyops.turboist.core.network

import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import java.util.concurrent.atomic.AtomicReference

/**
 * The address of the Turboist server this installation talks to.
 *
 * There is no compiled-in host: the product is self-hosted, so the user types
 * the address once and the whole HTTP stack has to follow it from that moment
 * on — including the client that was already built at process start. That is why
 * the address lives behind this holder rather than in Retrofit's `baseUrl`, which
 * is fixed for the lifetime of the client.
 *
 * Reads happen on every request from arbitrary threads; writes happen on the
 * connect screen. An atomic reference is all the coordination that needs.
 */
class ServerUrl(initial: String? = null) {
    private val current = AtomicReference(initial?.let(::normalize))

    /** The configured address, or `null` while the app has never been connected. */
    val value: HttpUrl?
        get() = current.get()

    /** True once an address has been accepted, which is what gates every authenticated call. */
    val isConfigured: Boolean
        get() = current.get() != null

    /**
     * Points the client at a new server.
     *
     * @throws IllegalArgumentException when the text is not an absolute http(s) URL.
     */
    fun set(url: String) {
        current.set(normalize(url) ?: throw IllegalArgumentException("server address must be an absolute http(s) URL"))
    }

    /** Forgets the address, which is what disconnecting from a server means. */
    fun clear() {
        current.set(null)
    }

    companion object {
        /**
         * Parses a user-entered address into the canonical form the request
         * rewriter expects: an absolute http(s) URL whose path ends in a slash, so
         * that endpoint paths append to it instead of replacing its last segment.
         *
         * A bare host is read as `https://host` — plain HTTP has to be asked for
         * explicitly, never fallen back to, or a typo would silently downgrade the
         * connection carrying the user's credentials.
         *
         * Returns `null` when the text names no usable address.
         */
        fun normalize(raw: String): HttpUrl? {
            val trimmed = raw.trim()
            if (trimmed.isEmpty()) return null
            val absolute = if (trimmed.contains("://")) trimmed else "https://$trimmed"
            val parsed = absolute.toHttpUrlOrNull() ?: return null
            if (parsed.scheme != "http" && parsed.scheme != "https") return null
            val path = parsed.encodedPath
            if (path.endsWith("/")) return parsed
            return parsed.newBuilder().encodedPath("$path/").build()
        }
    }
}
