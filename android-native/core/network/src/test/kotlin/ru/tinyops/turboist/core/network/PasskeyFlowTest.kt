package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.MockResponse
import ru.tinyops.turboist.core.model.ClientKind
import ru.tinyops.turboist.core.network.dto.PasskeyLoginBeginRequest
import ru.tinyops.turboist.core.network.dto.PasskeyLoginFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRegisterFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRenameRequest
import ru.tinyops.turboist.core.network.dto.optionsJson
import ru.tinyops.turboist.core.network.dto.passkeyCredentialOf
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The passkey endpoints driven against a mock server.
 *
 * A passkey assertion proves possession of the key *and* verification of the
 * user, so it stands in for both factors at once: the finish call answers with a
 * finished session even for an account that has a second factor configured, and
 * that is the property worth pinning — a client that went looking for a code
 * step after an assertion would add friction without adding a factor.
 */
class PasskeyFlowTest {
    private val challenge = "q-_9AAECAwQFBgcICQoLDA0ODw"

    private fun ceremonyJson(): String =
        """{"ceremonyId":"ceremony-1","options":{"publicKey":{"challenge":"$challenge","rpId":"todo.example.com"}}}"""

    @Test
    fun `a discoverable login carries no username and this client's own kind`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, ceremonyJson()))

            val begin = runBlocking { fixture.network.passkeys.loginBegin(PasskeyLoginBeginRequest()) }

            val request = fixture.server.takeRequest()
            assertEquals("/auth/passkey/login/begin", request.url.encodedPath)
            val body = TurboistJson.parseToJsonElement(request.body!!.utf8()).jsonObject
            assertEquals(ClientKind.ANDROID.wire, body.getValue("clientKind").jsonPrimitive.content)
            // Discoverable credentials answer with the account themselves, so the
            // request has nothing to say about who is signing in.
            assertNull(body["username"])
            assertEquals("ceremony-1", begin.ceremonyId)
            assertTrue(begin.optionsJson().contains(challenge))
        }
    }

    @Test
    fun `an assertion signs in an account with a second factor without a code step`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, sessionJson(totpEnabled = true)))

            val session =
                runBlocking {
                    fixture.network.passkeys.loginFinish(
                        PasskeyLoginFinishRequest(
                            ceremonyId = "ceremony-1",
                            credential = passkeyCredentialOf("""{"id":"Zm9v-_","response":{"signature":"MEUCIQ-_"}}"""),
                        ),
                    )
                }

            // A session, not a challenge: there is no ticket to carry into a
            // second step and nothing for the caller to branch on.
            assertEquals("access-1", session.access)
            assertEquals("refresh-1", session.refresh)
            assertTrue(session.user.totpEnabled)

            val body = TurboistJson.parseToJsonElement(fixture.server.takeRequest().body!!.utf8()).jsonObject
            assertEquals("ceremony-1", body.getValue("ceremonyId").jsonPrimitive.content)
            assertEquals(ClientKind.ANDROID.wire, body.getValue("clientKind").jsonPrimitive.content)
            assertEquals(
                "MEUCIQ-_",
                body.getValue("credential").jsonObject.getValue("response").jsonObject
                    .getValue("signature").jsonPrimitive.content,
            )
        }
    }

    @Test
    fun `a passkey login carries no session and never asks for a fresh token`() {
        val refreshes = AtomicInteger()
        val fixture =
            NetworkFixture(
                tokens = { "access-1" },
                refresher = {
                    refreshes.incrementAndGet()
                    "access-2"
                },
            )

        fixture.use {
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_INVALID, "assertion rejected"))

            assertFailsWith<ApiException> {
                runBlocking {
                    fixture.network.passkeys.loginFinish(
                        PasskeyLoginFinishRequest(ceremonyId = "ceremony-1", credential = passkeyCredentialOf("{}")),
                    )
                }
            }

            // A ceremony establishes a session rather than using one, and its
            // challenge is single-use: a retry would resend a `finish` the first
            // attempt already spent and answer with the wrong complaint.
            assertNull(fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
            assertEquals(0, refreshes.get())
        }
    }

    @Test
    fun `a replayed ceremony is reported as the refusal it is`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                errorResponse(400, ApiErrorCodes.PASSKEY_CEREMONY_INVALID, "ceremony expired"),
            )

            val failure =
                assertFailsWith<ApiException> {
                    runBlocking {
                        fixture.network.passkeys.loginFinish(
                            PasskeyLoginFinishRequest(ceremonyId = "spent", credential = passkeyCredentialOf("{}")),
                        )
                    }
                }

            assertEquals(ApiErrorCodes.PASSKEY_CEREMONY_INVALID, failure.code)
        }
    }

    @Test
    fun `enrolment posts the attestation with the name the user typed`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, ceremonyJson()))
            fixture.server.enqueue(
                jsonResponse(201, """{"id":3,"name":"Pixel","createdAt":"2026-08-18T10:00:00.000Z"}"""),
            )

            val stored =
                runBlocking {
                    fixture.network.passkeys.registerBegin()
                    fixture.network.passkeys.registerFinish(
                        PasskeyRegisterFinishRequest(
                            ceremonyId = "ceremony-1",
                            name = "Pixel",
                            credential =
                                passkeyCredentialOf(
                                    """{"id":"Zm9v-_","response":{"attestationObject":"o2M"}}""",
                                ),
                        ),
                    )
                }

            assertEquals(3L, stored.id)
            assertEquals("Pixel", stored.name)
            // Never used yet, which the list screen shows rather than inventing a date.
            assertNull(stored.lastUsedAt)

            fixture.server.takeRequest()
            val body = TurboistJson.parseToJsonElement(fixture.server.takeRequest().body!!.utf8()).jsonObject
            assertEquals("Pixel", body.getValue("name").jsonPrimitive.content)
        }
    }

    @Test
    fun `an unnamed enrolment leaves the naming to the server`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(201, """{"id":4,"name":"Passkey","createdAt":"2026-08-18T10:00:00.000Z"}"""),
            )

            runBlocking {
                fixture.network.passkeys.registerFinish(
                    PasskeyRegisterFinishRequest(ceremonyId = "ceremony-1", credential = passkeyCredentialOf("{}")),
                )
            }

            val body = TurboistJson.parseToJsonElement(fixture.server.takeRequest().body!!.utf8()).jsonObject
            assertNull(body["name"])
        }
    }

    @Test
    fun `the stored credentials are listed, renamed and removed by id`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    200,
                    """[{"id":3,"name":"Pixel","createdAt":"2026-08-18T10:00:00.000Z",
                       "lastUsedAt":"2026-08-18T12:30:00.000Z"}]""",
                ),
            )
            fixture.server.enqueue(
                jsonResponse(200, """{"id":3,"name":"Work phone","createdAt":"2026-08-18T10:00:00.000Z"}"""),
            )
            fixture.server.enqueue(MockResponse.Builder().code(204).build())

            val listed = runBlocking { fixture.network.passkeys.list() }
            assertEquals("Pixel", listed.single().name)
            assertEquals("2026-08-18T12:30:00.000Z", listed.single().lastUsedAt)

            val renamed = runBlocking { fixture.network.passkeys.rename(3, PasskeyRenameRequest("Work phone")) }
            assertEquals("Work phone", renamed.name)

            val removed = runBlocking { fixture.network.passkeys.remove(3) }
            assertEquals(204, removed.code())

            assertEquals("/api/v1/passkeys", fixture.server.takeRequest().url.encodedPath)
            assertEquals("/api/v1/passkeys/3", fixture.server.takeRequest().url.encodedPath)
            val delete = fixture.server.takeRequest()
            assertEquals("DELETE", delete.method)
            assertEquals("/api/v1/passkeys/3", delete.url.encodedPath)
        }
    }
}
