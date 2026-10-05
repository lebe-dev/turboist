package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.GetPublicKeyCredentialOption
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.optionsJson
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * What a running server actually starts a ceremony with, carried through this
 * app's unwrapping and into the platform's own request builders.
 *
 * The two payloads are files rather than literals because they are recordings,
 * not examples: each is the exact body a server configured as a relying party
 * answered `register/begin` and `login/begin` with — challenge, user handle,
 * algorithm list and all. `just android-native-passkey-preflight` brings such a
 * server up, asks it both questions again and fails if what comes back no longer
 * has the shape of these files, so the recording can be re-checked against the
 * server of the day instead of being taken on trust.
 *
 * They are pinned here because the platform builders parse what they are given
 * and refuse a shape they cannot read: an enrolment request needs a user with a
 * name to put on the sheet, and a login request needs a challenge. Everything
 * else about a passkey can be right and the ceremony will still never reach the
 * authenticator if these bytes do not survive the trip, and that failure only
 * shows up on a device — unless it is pinned here.
 *
 * The requests build Android parcels, so this runs against the framework rather
 * than on a bare JVM.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class ServerCeremonyOptionsTest {
    private fun recorded(name: String): String {
        val stream = javaClass.classLoader?.getResourceAsStream("passkey/$name")
        assertNotNull(stream, "the recorded ceremony $name is missing")
        return stream.bufferedReader().use { it.readText() }.trim()
    }

    private fun optionsOf(payload: String): String =
        TurboistJson.decodeFromString(PasskeyCeremonyDto.serializer(), payload).optionsJson()

    private val enrolmentCeremony = recorded("enrolment-ceremony.json")

    private val loginCeremony = recorded("login-ceremony.json")

    @Test
    fun `the server's enrolment options build a platform request as they are`() {
        val options = optionsOf(enrolmentCeremony)

        assertEquals(options, registrationRequest(options).requestJson)
        // A resident credential is what makes the later login usernameless: the
        // authenticator holds the account, so nobody has to type it.
        assertTrue(
            options.contains("\"residentKey\":\"required\""),
            "the enrolment options no longer ask for a discoverable credential",
        )
    }

    @Test
    fun `the server's login options build a platform request as they are`() {
        val options = optionsOf(loginCeremony)

        val option = assertionRequest(options).credentialOptions.single()
        assertIs<GetPublicKeyCredentialOption>(option)
        assertEquals(options, option.requestJson)
        // Nothing narrows the login to one credential or one account: the device
        // offers what it holds for this relying party and reports the choice.
        assertFalse(options.contains("allowCredentials"), "the login options named a credential to use")
        assertFalse(options.contains("\"user\""), "the login options named an account")
    }

    @Test
    fun `the browser wrapper is off by the time the platform sees the options`() {
        // The server writes the shape a browser calls its credential API with;
        // the platform here takes the inner object, and a request built around
        // the wrapper would be refused for having no challenge.
        assertFalse(optionsOf(enrolmentCeremony).contains("publicKey"))
        assertFalse(optionsOf(loginCeremony).contains("publicKey"))
    }
}
