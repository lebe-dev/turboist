package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.CreatePasswordResponse
import androidx.credentials.CreatePublicKeyCredentialResponse
import androidx.credentials.GetPublicKeyCredentialOption
import androidx.credentials.PasswordCredential
import androidx.credentials.PublicKeyCredential
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs

/**
 * The edges where this app meets Credential Manager.
 *
 * The prompt itself belongs to the platform and cannot be raised off a device,
 * but the two things on either side of it can: what is handed to the platform,
 * and what is made of the answer. Both are where a passkey silently breaks —
 * options rewritten on the way in no longer match the signature the server will
 * verify, and an answer of the wrong kind posted to a ceremony endpoint can only
 * be refused, with nothing left to say why.
 *
 * The requests build Android parcels, so this runs against the framework rather
 * than on a bare JVM.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class CredentialManagerRequestTest {
    private val assertionOptions =
        """
        {"challenge":"q-_9AAECAwQFBgcICQoLDA0ODw","rpId":"todo.example.com",
         "userVerification":"required","timeout":60000}
        """.trimIndent()

    private val registrationOptions =
        """
        {"challenge":"q-_9AAECAwQFBgcICQoLDA0ODw","rp":{"id":"todo.example.com","name":"Turboist"},
         "user":{"id":"AQIDBA","name":"alice","displayName":"alice"},
         "pubKeyCredParams":[{"type":"public-key","alg":-7}],
         "authenticatorSelection":{"residentKey":"required","userVerification":"required"}}
        """.trimIndent()

    @Test
    fun `the server's assertion options reach the platform byte for byte`() {
        val option = assertionRequest(assertionOptions).credentialOptions.single()

        assertIs<GetPublicKeyCredentialOption>(option)
        assertEquals(assertionOptions, option.requestJson)
    }

    @Test
    fun `the server's enrolment options reach the platform byte for byte`() {
        assertEquals(registrationOptions, registrationRequest(registrationOptions).requestJson)
    }

    @Test
    fun `an assertion is passed on exactly as the authenticator wrote it`() {
        // Base64url is the alphabet the whole ceremony is written in; a value
        // carrying its two distinguishing characters proves nothing re-encoded it.
        val answer = """{"id":"Zm9v-_","rawId":"Zm9v-_","type":"public-key","response":{"signature":"MEUCIQ-_"}}"""

        assertEquals(answer, assertionAnswerOf(PublicKeyCredential(answer)))
    }

    @Test
    fun `an attestation is passed on exactly as the authenticator wrote it`() {
        val answer = """{"id":"Zm9v-_","rawId":"Zm9v-_","type":"public-key","response":{"attestationObject":"o2M-_"}}"""

        assertEquals(answer, registrationAnswerOf(CreatePublicKeyCredentialResponse(answer)))
    }

    @Test
    fun `a credential of another kind is refused instead of being posted as an assertion`() {
        // A provider may answer with a saved password. Sending it to the ceremony
        // endpoint would be refused there, far from the reason.
        assertFailsWith<IllegalArgumentException> {
            assertionAnswerOf(PasswordCredential(id = "alice", password = "hunter2"))
        }
        assertFailsWith<IllegalArgumentException> {
            registrationAnswerOf(CreatePasswordResponse())
        }
    }
}
