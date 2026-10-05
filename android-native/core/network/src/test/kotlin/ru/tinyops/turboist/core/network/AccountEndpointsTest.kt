package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.MockResponse
import ru.tinyops.turboist.core.network.dto.CreateApiTokenRequest
import ru.tinyops.turboist.core.network.dto.TotpCodeRequest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The account's administrative endpoints driven against a mock server.
 *
 * They are the surfaces this client deliberately does not replicate, so what is
 * worth pinning is the wire itself: the paths, the one response that carries a
 * secret, and the two refusals a client has to tell apart — a wrong code, and a
 * deployment that never brought the second factor up at all.
 */
class AccountEndpointsTest {
    @Test
    fun `the session list arrives with the current one flagged`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    200,
                    """
                    [{"id":12,"clientKind":"web","userAgent":"Mozilla/5.0","displayName":"Chrome on macOS",
                      "ipAddress":"203.0.113.10","createdAt":"2026-05-01T10:00:00.000Z",
                      "lastUsedAt":"2026-05-24T09:30:00.000Z","isCurrent":true}]
                    """.trimIndent().replace("\n", ""),
                ),
            )

            val sessions = runBlocking { fixture.network.sessions.list() }

            assertEquals("/api/v1/sessions", fixture.server.takeRequest().url.encodedPath)
            val session = sessions.single()
            assertEquals(12L, session.id)
            assertEquals("Chrome on macOS", session.displayName)
            assertEquals("203.0.113.10", session.ipAddress)
            assertTrue(session.isCurrent)
        }
    }

    @Test
    fun `revoking a session names it in the path and answers with nothing`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(MockResponse.Builder().code(204).build())

            val response = runBlocking { fixture.network.sessions.revoke(12) }

            val request = fixture.server.takeRequest()
            assertEquals("DELETE", request.method)
            assertEquals("/api/v1/sessions/12", request.url.encodedPath)
            assertTrue(response.isSuccessful)
        }
    }

    @Test
    fun `a session that is already gone is a refusal, not an empty answer`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(404, ApiErrorCodes.NOT_FOUND, "no such session"))

            val failure =
                assertFailsWith<ApiException.Business> { runBlocking { fixture.network.sessions.revoke(99) } }

            assertEquals(ApiErrorCodes.NOT_FOUND, failure.code)
        }
    }

    @Test
    fun `minting a token sends the scopes and answers with the one plaintext there will ever be`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    201,
                    """
                    {"id":1,"name":"n8n","scopes":["tasks:read","tasks:write"],
                     "token":"secret-value","createdAt":"2026-05-01T10:00:00.000Z"}
                    """.trimIndent().replace("\n", ""),
                ),
            )

            val minted =
                runBlocking {
                    fixture.network.apiTokens.create(
                        CreateApiTokenRequest(name = "n8n", scopes = listOf("tasks:read", "tasks:write")),
                    )
                }

            val request = fixture.server.takeRequest()
            assertEquals("/api/v1/api-tokens", request.url.encodedPath)
            val body = TurboistJson.parseToJsonElement(request.body!!.utf8()).jsonObject
            assertEquals("n8n", body.getValue("name").jsonPrimitive.content)
            assertEquals(
                listOf("tasks:read", "tasks:write"),
                body.getValue("scopes").jsonArray.map { it.jsonPrimitive.content },
            )
            assertEquals("secret-value", minted.token)
        }
    }

    @Test
    fun `a listed token carries no secret at all`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(200, """[{"id":1,"name":"n8n","scopes":["*"],"createdAt":"2026-05-01T10:00:00.000Z"}]"""),
            )

            val tokens = runBlocking { fixture.network.apiTokens.list() }

            assertEquals("/api/v1/api-tokens", fixture.server.takeRequest().url.encodedPath)
            // The listed shape has nowhere to put one, which is the point: the
            // server kept only a hash and could not send it back if it wanted to.
            assertEquals(listOf("*"), tokens.single().scopes)
            assertNull(
                TurboistJson.parseToJsonElement(
                    """{"id":1,"name":"n8n","scopes":["*"],"createdAt":"2026-05-01T10:00:00.000Z"}""",
                ).jsonObject["token"],
            )
        }
    }

    @Test
    fun `an enrolment answers with the secret in the three forms an app can take it`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    200,
                    """
                    {"secret":"JBSWY3DPEHPK3PXP",
                     "otpauthUrl":"otpauth://totp/Turboist:alice?secret=JBSWY3DPEHPK3PXP&issuer=Turboist",
                     "qrPngBase64":"iVBORw0KGgo="}
                    """.trimIndent().replace("\n", ""),
                ),
            )

            val enrolment = runBlocking { fixture.network.totp.setup() }

            assertEquals("/auth/totp/setup", fixture.server.takeRequest().url.encodedPath)
            assertEquals("JBSWY3DPEHPK3PXP", enrolment.secret)
            assertTrue(enrolment.otpauthUrl.startsWith("otpauth://totp/"))
            assertTrue(enrolment.qrPngBase64.isNotEmpty())
        }
    }

    @Test
    fun `confirming answers with the recovery codes, which arrive exactly once`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"recoveryCodes":["ABCDEFGHJK","KLMNOPQRST"]}"""))

            val codes = runBlocking { fixture.network.totp.confirm(TotpCodeRequest("123456")) }

            val request = fixture.server.takeRequest()
            assertEquals("/auth/totp/confirm", request.url.encodedPath)
            assertEquals(
                "123456",
                TurboistJson.parseToJsonElement(request.body!!.utf8()).jsonObject.getValue("code")
                    .jsonPrimitive.content,
            )
            assertEquals(listOf("ABCDEFGHJK", "KLMNOPQRST"), codes.recoveryCodes)
        }
    }

    @Test
    fun `a wrong code is a refusal a client can name`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.TOTP_INVALID_CODE, "invalid code"))

            val failure =
                assertFailsWith<ApiException.Auth> {
                    runBlocking { fixture.network.totp.confirm(TotpCodeRequest("000000")) }
                }

            // Not a session problem: the code is what was rejected, and the
            // request must not be retried with a fresh access token.
            assertEquals(ApiErrorCodes.TOTP_INVALID_CODE, failure.code)
            assertEquals(1, fixture.server.requestCount)
        }
    }

    @Test
    fun `a deployment without a second factor has no such route`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(404, ApiErrorCodes.NOT_FOUND, "not found"))

            val failure =
                assertFailsWith<ApiException.Business> { runBlocking { fixture.network.totp.setup() } }

            // The status is what identifies it: an absent route is the router's
            // own refusal and carries no error code of the feature's own.
            assertEquals(404, failure.status)
        }
    }
}
