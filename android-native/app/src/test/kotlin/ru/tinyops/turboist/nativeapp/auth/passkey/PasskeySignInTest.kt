package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.exceptions.GetCredentialCancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.PasskeyConfigDto
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.core.network.dto.UserDto
import ru.tinyops.turboist.nativeapp.auth.CleartextPolicy
import ru.tinyops.turboist.nativeapp.auth.FakeAuthGateway
import ru.tinyops.turboist.nativeapp.auth.FakeLocalReplica
import ru.tinyops.turboist.nativeapp.auth.FakeNetworkAvailability
import ru.tinyops.turboist.nativeapp.auth.FakeRefreshTokenStore
import ru.tinyops.turboist.nativeapp.auth.FakeServerAddressStore
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.auth.SessionTokens
import ru.tinyops.turboist.nativeapp.session.SessionState
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Signing in with a passkey, with the server and the device replaced by
 * stand-ins.
 *
 * The rule the whole feature rests on is that an assertion is *already* two
 * factors: the authenticator holds the key and verified the user, so a code step
 * afterwards would add friction without adding a factor. It is asserted here
 * against an account that has a second factor configured, because that is the
 * only case where getting it wrong is visible.
 *
 * The other half is that nothing a ceremony does can cost the user their way in:
 * a dismissed sheet, a device with no credential and a server that refuses the
 * assertion all leave the password form exactly as it was.
 */
class PasskeySignInTest {
    private val gateway = FakeAuthGateway()
    private val refreshTokens = FakeRefreshTokenStore()
    private val accessTokens = SessionTokens()

    private fun manager() =
        SessionManager(
            gateway = gateway,
            accessTokens = accessTokens,
            refreshTokens = refreshTokens,
            serverAddresses = FakeServerAddressStore(),
            serverUrl = ServerUrl(),
            replica = FakeLocalReplica(),
            networks = FakeNetworkAvailability(),
            cleartext = CleartextPolicy(allowed = false),
            scope = CoroutineScope(Dispatchers.Unconfined),
        )

    /** An authenticator that answers with whatever the test scripted. */
    private class ScriptedAuthenticator(
        private val answer: (String) -> String,
    ) : PasskeyAuthenticator {
        var optionsSeen: String? = null

        override suspend fun assertion(optionsJson: String): String {
            optionsSeen = optionsJson
            return answer(optionsJson)
        }

        override suspend fun registration(optionsJson: String): String = answer(optionsJson)
    }

    private fun answering(json: String) = ScriptedAuthenticator { json }

    private fun refusing(error: Throwable) = ScriptedAuthenticator { throw error }

    private val assertion =
        """
        {"id":"Zm9v-_","rawId":"Zm9v-_","type":"public-key",
         "response":{"clientDataJSON":"eyJ0-XBl","authenticatorData":"SZYN5Y_g","signature":"MEUCIQ-_",
         "userHandle":"AQIDBA"}}
        """.trimIndent()

    @Test
    fun `an assertion signs in an account with a second factor, with no code step`() =
        runTest {
            gateway.passkeyLoginResult = {
                SessionDto(
                    access = "access-passkey",
                    refresh = "refresh-passkey",
                    user = UserDto(id = 1, username = "alice", totpEnabled = true),
                )
            }
            val session = manager()

            val outcome = session.signInWithPasskey(answering(assertion))

            assertEquals(PasskeyOutcome.SignedIn, outcome)
            assertEquals(SessionState.LoggedIn, session.state.value)
            assertEquals("access-passkey", accessTokens.accessToken())
            // The rotation is stored before anything uses the session, exactly as
            // a password sign-in stores it.
            assertEquals("refresh-passkey", refreshTokens.stored())
        }

    @Test
    fun `the device is handed the server's own options, without the browser wrapper`() =
        runTest {
            val authenticator = answering(assertion)

            manager().signInWithPasskey(authenticator)

            val handedOver = TurboistJson.parseToJsonElement(authenticator.optionsSeen.orEmpty()).jsonObject
            assertNull(handedOver["publicKey"])
            assertEquals("q-_9AAECAwQFBgcICQoLDA0ODw", handedOver.getValue("challenge").jsonPrimitive.content)
            assertEquals("todo.example.com", handedOver.getValue("rpId").jsonPrimitive.content)
        }

    @Test
    fun `the authenticator's answer reaches the server unchanged`() =
        runTest {
            manager().signInWithPasskey(answering(assertion))

            val posted = assertNotNull(gateway.assertedCredential)
            assertEquals(listOf("ceremony-1"), gateway.passkeyCeremonyIds)
            val response = posted.getValue("response").jsonObject
            assertEquals("MEUCIQ-_", response.getValue("signature").jsonPrimitive.content)
            // The user handle is what makes the login usernameless; dropping it
            // would leave the server with no account to resolve.
            assertEquals("AQIDBA", response.getValue("userHandle").jsonPrimitive.content)
        }

    @Test
    fun `a dismissed sheet is reported as a choice and costs nothing`() =
        runTest {
            val session = manager()

            val outcome = session.signInWithPasskey(refusing(GetCredentialCancellationException()))

            // Closing the sheet is a choice, so it is reported as one — and a
            // ceremony that did not finish leaves the session and the stored
            // token exactly as they were.
            assertEquals(PasskeyOutcome.Failed(PasskeyProblem.Cancelled), outcome)
            assertEquals(SessionState.Connecting, session.state.value)
            assertNull(accessTokens.accessToken())
            assertNull(refreshTokens.stored())
            assertTrue(gateway.passkeyCeremonyIds.isEmpty())
        }

    @Test
    fun `a server that refuses the assertion does not send the user looking for a code`() =
        runTest {
            gateway.passkeyLoginResult = {
                throw ApiException.from(
                    401,
                    ApiErrorBody(code = ApiErrorCodes.AUTH_INVALID, message = "assertion rejected"),
                )
            }
            val session = manager()

            val outcome = session.signInWithPasskey(answering(assertion))

            assertEquals(PasskeyOutcome.Failed(PasskeyProblem.Refused), outcome)
            assertEquals(SessionState.Connecting, session.state.value)
            assertNull(accessTokens.accessToken())
        }

    @Test
    fun `an unfinished ceremony is a refusal rather than a crash`() =
        runTest {
            val outcome = manager().signInWithPasskey(answering("not a credential"))

            assertEquals(PasskeyOutcome.Failed(PasskeyProblem.Failed), outcome)
        }

    @Test
    fun `the offer is made only by a server that has passkeys and something enrolled`() =
        runTest {
            val session = manager()

            gateway.passkeyOffer = PasskeyConfigDto(enabled = true, available = true)
            session.refreshPasskeyOffer()
            assertTrue(session.passkeySignInOffered.value)

            // Enabled but nothing enrolled: the button could only ever end in an
            // explanation, so it is not shown.
            gateway.passkeyOffer = PasskeyConfigDto(enabled = true, available = false)
            session.refreshPasskeyOffer()
            assertFalse(session.passkeySignInOffered.value)

            gateway.passkeyOffer = PasskeyConfigDto(enabled = false, available = true)
            session.refreshPasskeyOffer()
            assertFalse(session.passkeySignInOffered.value)
        }

    @Test
    fun `a server that cannot answer leaves the password form alone`() =
        runTest {
            val session = manager()
            session.refreshPasskeyOffer()
            assertTrue(session.passkeySignInOffered.value)

            gateway.reachable = false
            session.refreshPasskeyOffer()

            assertFalse(session.passkeySignInOffered.value)
        }
}
