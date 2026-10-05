package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.exceptions.GetCredentialCancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.Response
import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.api.PasskeyApi
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.PasskeyDto
import ru.tinyops.turboist.core.network.dto.PasskeyLoginBeginRequest
import ru.tinyops.turboist.core.network.dto.PasskeyLoginFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRegisterFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRenameRequest
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.nativeapp.auth.passkey.ui.PasskeysViewModel
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Managing the credentials an account can sign in with, with the server and the
 * device replaced by stand-ins.
 *
 * Three rules are worth pinning here rather than leaving to the screen.
 *
 * A list that could not be fetched is *not* an account with no passkeys. Those
 * two states look alike and mean opposite things: one says "nothing can sign in
 * but your password", the other says "we do not know". Showing the first when
 * the second is true invites the user to enrol a duplicate, or to believe a
 * credential they revoked is gone when nobody has confirmed it.
 *
 * A second ceremony cannot start while one is out. The platform sheet is modal,
 * so the second call would be answered by whatever the user did to the first
 * one, and the account would end up with a credential nobody chose to create.
 *
 * Enrolment carries JSON and nothing else: the server's options reach the device
 * unwrapped, and the device's answer reaches the server byte for byte. The
 * attestation is signed over exactly those bytes, so anything rewritten in
 * between is verified against nothing.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class PasskeyManagementTest {
    private val dispatcher = StandardTestDispatcher()
    private val api = FakePasskeyApi()
    private val registry = PasskeyRegistry(api)

    @Before
    fun useTestDispatcher() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun releaseDispatcher() {
        Dispatchers.resetMain()
    }

    private fun stored(
        id: Long,
        name: String,
    ) = PasskeyDto(id = id, name = name, createdAt = "2026-08-18T10:00:00.000Z")

    /** What the device answers a ceremony with; the shape the server verifies. */
    private val credential =
        """
        {"id":"Zm9v-_","rawId":"Zm9v-_","type":"public-key",
         "response":{"clientDataJSON":"eyJ0-XBl","attestationObject":"o2NmbXQ-_"}}
        """.trimIndent()

    private fun answering(answer: String = credential) = ScriptedAuthenticator { answer }

    private fun refusing(error: Throwable) = ScriptedAuthenticator { throw error }

    @Test
    fun `the account's credentials arrive and the screen stops waiting`() =
        runTest(dispatcher) {
            api.stored += listOf(stored(1, "Pixel"), stored(2, "Security key"))

            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            val state = model.state.value
            assertFalse(state.loading)
            assertFalse(state.unreachable)
            assertEquals(listOf("Pixel", "Security key"), state.passkeys.map { it.name })
        }

    @Test
    fun `a list that could not be fetched is said to be missing, not empty`() =
        runTest(dispatcher) {
            api.listFailure = ApiException.Network("the request did not reach the server")

            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            val failed = model.state.value
            assertTrue(failed.unreachable, "an unfetchable list was reported as an account with no passkeys")
            assertFalse(failed.loading)
            assertEquals(PasskeyProblem.Unreachable, failed.problem)

            // And a retry that works clears it rather than leaving the warning up.
            api.listFailure = null
            api.stored += stored(1, "Pixel")
            model.load()
            advanceUntilIdle()

            assertFalse(model.state.value.unreachable)
            assertEquals(listOf("Pixel"), model.state.value.passkeys.map { it.name })
        }

    @Test
    fun `enrolling carries the server's options to the device and the answer back untouched`() =
        runTest(dispatcher) {
            val authenticator = answering()
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.add(authenticator, "  Pixel  ")
            advanceUntilIdle()

            // The device is handed the options the server wrote, without the
            // wrapper a browser's credential API is called with.
            val handedOver = TurboistJson.parseToJsonElement(authenticator.optionsSeen.orEmpty()).jsonObject
            assertNull(handedOver["publicKey"])
            assertEquals("Q0hBTExFTkdF-_", handedOver["challenge"]?.jsonPrimitive?.content)

            val posted = assertNotNull(api.registerFinishSeen)
            assertEquals("ceremony-register", posted.ceremonyId)
            assertEquals(
                TurboistJson.parseToJsonElement(credential).jsonObject,
                posted.credential,
                "the device's answer was rewritten on the way to the server",
            )
            // A label the user typed travels trimmed, and the stored credential
            // the server answered with is what the list then shows.
            assertEquals("Pixel", posted.name)
            assertEquals(listOf("Pixel"), model.state.value.passkeys.map { it.name })
            assertTrue(model.state.value.added)
            assertFalse(model.state.value.busy)
        }

    @Test
    fun `a name left blank lets the server label the credential`() =
        runTest(dispatcher) {
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.add(answering(), "   ")
            advanceUntilIdle()

            // Not an empty string: a placeholder invented here would overwrite
            // whatever the server would have called it.
            assertNull(assertNotNull(api.registerFinishSeen).name)
        }

    @Test
    fun `a second ceremony cannot start while one is out`() =
        runTest(dispatcher) {
            val held = CompletableDeferred<String>()
            val first = ScriptedAuthenticator { held.await() }
            val second = answering()
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.add(first, "Pixel")
            runCurrent()
            assertTrue(model.state.value.busy)
            assertTrue(model.state.value.awaitingDevice)

            model.add(second, "Tablet")
            model.rename(1, "Renamed")
            model.remove(1)
            runCurrent()

            assertNull(second.optionsSeen, "a second sheet was opened over the first")
            assertEquals(1, api.registerBeginCalls)
            assertTrue(api.renameSeen.isEmpty())
            assertTrue(api.removed.isEmpty())

            held.complete(credential)
            advanceUntilIdle()

            assertEquals(listOf("Pixel"), model.state.value.passkeys.map { it.name })
            assertFalse(model.state.value.busy)
            assertFalse(model.state.value.awaitingDevice)
        }

    @Test
    fun `a ceremony the user dismissed changes nothing and lets them try again`() =
        runTest(dispatcher) {
            api.stored += stored(1, "Pixel")
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.add(refusing(GetCredentialCancellationException()), "Tablet")
            advanceUntilIdle()

            val state = model.state.value
            assertEquals(PasskeyProblem.Cancelled, state.problem)
            assertEquals(listOf("Pixel"), state.passkeys.map { it.name })
            assertFalse(state.added)
            // The lock is released whatever happened, or the screen would be dead
            // after the first dismissal.
            assertFalse(state.busy)
            assertFalse(state.awaitingDevice)

            model.dismissMessage()
            assertNull(model.state.value.problem)
        }

    @Test
    fun `renaming replaces the row in place, and a blank name is not sent`() =
        runTest(dispatcher) {
            api.stored += listOf(stored(1, "Pixel"), stored(2, "Security key"))
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.rename(2, "  Yubikey  ")
            advanceUntilIdle()

            assertEquals(listOf("Pixel", "Yubikey"), model.state.value.passkeys.map { it.name })
            assertEquals(listOf(2L to "Yubikey"), api.renameSeen)

            model.rename(1, "   ")
            advanceUntilIdle()

            // An empty label would leave a credential nobody can tell apart from
            // the others in the list.
            assertEquals(listOf(2L to "Yubikey"), api.renameSeen)
            assertEquals(listOf("Pixel", "Yubikey"), model.state.value.passkeys.map { it.name })
        }

    @Test
    fun `the last credential may be removed, because the password is the way back in`() =
        runTest(dispatcher) {
            api.stored += stored(1, "Pixel")
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.remove(1)
            advanceUntilIdle()

            assertEquals(listOf(1L), api.removed)
            assertTrue(model.state.value.passkeys.isEmpty())
            assertNull(model.state.value.problem)
            // An account with no passkeys is a real answer, not a failed load.
            assertFalse(model.state.value.unreachable)
        }

    @Test
    fun `a refused removal leaves the credential on the list it is still on`() =
        runTest(dispatcher) {
            api.stored += stored(1, "Pixel")
            api.removeFailure =
                ApiException.from(404, ApiErrorBody(code = ApiErrorCodes.NOT_FOUND, message = "no such passkey"))
            val model = PasskeysViewModel(registry)
            advanceUntilIdle()

            model.remove(1)
            advanceUntilIdle()

            // Nothing is dropped on the strength of a call that failed: the list
            // answers which devices can sign in, and guessing at it is the one
            // thing this screen must not do.
            assertEquals(listOf("Pixel"), model.state.value.passkeys.map { it.name })
            assertEquals(PasskeyProblem.Unsupported, model.state.value.problem)
        }
}

/** An authenticator that answers with whatever the test scripted. */
private class ScriptedAuthenticator(
    private val answer: suspend (String) -> String,
) : PasskeyAuthenticator {
    var optionsSeen: String? = null

    override suspend fun assertion(optionsJson: String): String = registration(optionsJson)

    override suspend fun registration(optionsJson: String): String {
        optionsSeen = optionsJson
        return answer(optionsJson)
    }
}

/**
 * A server that keeps the account's credentials in a list.
 *
 * The ceremony options are the shape a real one sends — wrapped for a browser's
 * credential API — so the unwrapping is exercised rather than assumed.
 */
private class FakePasskeyApi : PasskeyApi {
    val stored = mutableListOf<PasskeyDto>()
    val renameSeen = mutableListOf<Pair<Long, String>>()
    val removed = mutableListOf<Long>()
    var listFailure: Throwable? = null
    var removeFailure: Throwable? = null
    var registerBeginCalls = 0
    var registerFinishSeen: PasskeyRegisterFinishRequest? = null

    override suspend fun loginBegin(body: PasskeyLoginBeginRequest): PasskeyCeremonyDto =
        throw UnsupportedOperationException("this fake stands in for the management calls only")

    override suspend fun loginFinish(body: PasskeyLoginFinishRequest): SessionDto =
        throw UnsupportedOperationException("this fake stands in for the management calls only")

    override suspend fun list(): List<PasskeyDto> {
        listFailure?.let { throw it }
        return stored.toList()
    }

    override suspend fun registerBegin(): PasskeyCeremonyDto {
        registerBeginCalls++
        return TurboistJson.decodeFromString(
            PasskeyCeremonyDto.serializer(),
            """
            {"ceremonyId":"ceremony-register","options":{"publicKey":{
            "rp":{"name":"Turboist","id":"todo.example.com"},
            "user":{"name":"alice","displayName":"alice","id":"VVNFUg-_"},
            "challenge":"Q0hBTExFTkdF-_",
            "pubKeyCredParams":[{"type":"public-key","alg":-7}],
            "authenticatorSelection":{"residentKey":"required","userVerification":"preferred"}}}}
            """.trimIndent().replace("\n", ""),
        )
    }

    override suspend fun registerFinish(body: PasskeyRegisterFinishRequest): PasskeyDto {
        registerFinishSeen = body
        val credential =
            PasskeyDto(
                id = (stored.maxOfOrNull { it.id } ?: 0) + 1,
                name = body.name ?: "Passkey",
                createdAt = "2026-08-20T10:00:00.000Z",
            )
        stored += credential
        return credential
    }

    override suspend fun rename(
        id: Long,
        body: PasskeyRenameRequest,
    ): PasskeyDto {
        renameSeen += id to body.name
        val index = stored.indexOfFirst { it.id == id }
        val renamed = stored[index].copy(name = body.name)
        stored[index] = renamed
        return renamed
    }

    override suspend fun remove(id: Long): Response<Unit> {
        removeFailure?.let { throw it }
        removed += id
        stored.removeAll { it.id == id }
        return Response.success(Unit)
    }
}
