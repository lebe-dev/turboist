package ru.tinyops.turboist.core.network

import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import ru.tinyops.turboist.core.network.auth.AccessTokenRefresher
import ru.tinyops.turboist.core.network.auth.AccessTokenSource
import ru.tinyops.turboist.core.network.dto.CompleteTaskRequest
import ru.tinyops.turboist.core.network.dto.LoginRequest

/** A JSON body with the given status, plus any extra headers the case needs. */
internal fun jsonResponse(
    code: Int,
    body: String,
    headers: Map<String, String> = emptyMap(),
): MockResponse {
    val builder =
        MockResponse.Builder()
            .code(code)
            .setHeader("Content-Type", "application/json")
            .body(body)
    headers.forEach { (name, value) -> builder.setHeader(name, value) }
    return builder.build()
}

/** The `{error: {...}}` envelope the API answers every failure with. */
internal fun errorResponse(
    code: Int,
    errorCode: String,
    message: String = "something went wrong",
    details: String? = null,
): MockResponse {
    val detailsPart = if (details == null) "" else ""","details":$details"""
    return jsonResponse(code, """{"error":{"code":"$errorCode","message":"$message"$detailsPart}}""")
}

/**
 * A stack pointed at a local mock server.
 *
 * Everything a test needs to drive the real interceptor chain: nothing here is a
 * stand-in for the HTTP layer, only for the server on the other end of it.
 */
internal class NetworkFixture(
    tokens: AccessTokenSource = AccessTokenSource.None,
    refresher: AccessTokenRefresher = AccessTokenRefresher.None,
    newIdempotencyKey: () -> String = { "fixed-key" },
) : AutoCloseable {
    val server: MockWebServer = MockWebServer().apply { start() }
    val serverUrl: ServerUrl = ServerUrl(server.url("/").toString())
    val network: TurboistNetwork =
        TurboistNetwork.create(
            serverUrl = serverUrl,
            tokens = tokens,
            refresher = refresher,
            newIdempotencyKey = newIdempotencyKey,
        )

    override fun close() {
        server.close()
    }
}

/** A completion with no explicit moment — the shape a write made while online has. */
internal fun completeBody(): CompleteTaskRequest = CompleteTaskRequest()

internal fun loginBody(): LoginRequest = LoginRequest(username = "alice", password = "secret")

/** A completed session as the server writes it. */
internal fun sessionJson(
    access: String = "access-1",
    totpEnabled: Boolean = false,
): String =
    """{"access":"$access","refresh":"refresh-1","user":{"id":1,"username":"alice","totpEnabled":$totpEnabled}}"""
