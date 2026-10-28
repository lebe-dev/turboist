package ru.tinyops.turboist.core.network.http

import okhttp3.Interceptor
import okhttp3.Response
import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorEnvelope
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.TurboistJson
import java.io.IOException

/**
 * Turns every failure — a refused request or an unreachable server — into one
 * typed exception family.
 *
 * Callers of this module never see an HTTP status code or a raw transport
 * exception. They see an [ApiException] whose subclass already answers what to do
 * next, and whose `code` and `details` still carry what the server said. That is
 * the whole point of doing it here rather than at each call site: there is one
 * place where "409 with this code means the task is blocked by these ids" is
 * written down.
 *
 * It is the outermost interceptor, so the layers beneath it still see the raw
 * responses they need — most importantly the 401 that the token refresh reacts
 * to, which must not have been converted into an exception by the time it gets
 * there.
 */
class ErrorMappingInterceptor : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val response =
            try {
                chain.proceed(chain.request())
            } catch (e: ApiException) {
                throw e
            } catch (e: IOException) {
                throw ApiException.Network(e.message ?: "the request did not reach the server", e)
            }
        if (response.isSuccessful) {
            if (!response.isWebPage()) return response
            response.close()
            throw ApiException.Network(NOT_FROM_SERVER)
        }

        val body = response.readErrorBody()
        response.close()
        throw ApiException.from(response.code, body)
    }

    /**
     * True when a success arrived as a web page.
     *
     * The server answers every API call in JSON — an unknown path included,
     * which gets the JSON not-found envelope rather than the web app — so a page
     * here came from something standing between the device and the server: the
     * sign-in page of a hotel or train network, typically, which answers every
     * request with itself until the user has been through it. Reading that as
     * the server's own success would hand a parser HTML and count the failure
     * against a write the server never saw.
     */
    private fun Response.isWebPage(): Boolean = header("Content-Type")?.startsWith(HTML_TYPE, ignoreCase = true) == true

    /**
     * Reads the error envelope, tolerating anything that is not one. A proxy
     * returning an HTML 502, or a truncated body, still has to produce a usable
     * exception — with the status, which is the part that always exists.
     */
    private fun Response.readErrorBody(): ApiErrorBody? {
        val text =
            try {
                peekBody(MAX_ERROR_BODY_BYTES).string()
            } catch (_: IOException) {
                return null
            }
        if (text.isBlank()) return null
        return try {
            TurboistJson.decodeFromString(ApiErrorEnvelope.serializer(), text).error
        } catch (_: Exception) {
            null
        }
    }

    private companion object {
        const val HTML_TYPE = "text/html"
        const val NOT_FROM_SERVER =
            "the answer came from the network rather than the server; it may need signing in to"
    }
}
