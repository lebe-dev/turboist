package ru.tinyops.turboist.core.network

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.optionsJson
import ru.tinyops.turboist.core.network.dto.passkeyCredentialOf
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * The two edges of a passkey ceremony, where the JSON changes hands.
 *
 * Every binary member — the challenge, the user handle, credential ids, the
 * signature — travels as a base64url string and must arrive at the other side
 * byte-identical. The URL alphabet (`-` and `_`) and the missing padding are
 * exactly what a well-meant re-encoding would destroy, and the failure would be
 * a signature that verifies nowhere, so both directions are pinned here.
 */
class PasskeyCeremonyTest {
    /** A challenge that uses both characters the URL alphabet has and standard base64 does not. */
    private val challenge = "q-_9AAECAwQFBgcICQoLDA0ODw"

    private fun ceremony(options: String): PasskeyCeremonyDto =
        TurboistJson.decodeFromString(
            PasskeyCeremonyDto.serializer(),
            """{"ceremonyId":"ceremony-1","options":$options}""",
        )

    @Test
    fun `the browser wrapper is unwrapped for the platform authenticator`() {
        val dto = ceremony("""{"publicKey":{"challenge":"$challenge","rpId":"todo.example.com"}}""")

        val handedOver = TurboistJson.parseToJsonElement(dto.optionsJson()).jsonObject

        // The platform credential API takes the options themselves, so the member
        // a browser call would be wrapped in must not still be there.
        assertEquals(null, handedOver["publicKey"])
        assertEquals(challenge, handedOver.getValue("challenge").jsonPrimitive.content)
        assertEquals("todo.example.com", handedOver.getValue("rpId").jsonPrimitive.content)
    }

    @Test
    fun `options sent without the wrapper are handed over as they are`() {
        val dto = ceremony("""{"challenge":"$challenge","userVerification":"preferred"}""")

        val handedOver = TurboistJson.parseToJsonElement(dto.optionsJson()).jsonObject

        assertEquals(challenge, handedOver.getValue("challenge").jsonPrimitive.content)
        assertEquals("preferred", handedOver.getValue("userVerification").jsonPrimitive.content)
    }

    @Test
    fun `nothing in the options is decoded, renamed or dropped on the way out`() {
        val dto =
            ceremony(
                """
                {"publicKey":{"challenge":"$challenge",
                 "user":{"id":"AQIDBA","name":"alice","displayName":"Alice"},
                 "excludeCredentials":[{"id":"Zm9v-_","type":"public-key","transports":["internal","hybrid"]}],
                 "authenticatorSelection":{"residentKey":"required","userVerification":"preferred"},
                 "timeout":300000}}
                """.trimIndent(),
            )

        val handedOver = TurboistJson.parseToJsonElement(dto.optionsJson()).jsonObject

        assertEquals("AQIDBA", handedOver.getValue("user").jsonObject.getValue("id").jsonPrimitive.content)
        val excluded = handedOver.getValue("excludeCredentials").jsonArray.single().jsonObject
        assertEquals("Zm9v-_", excluded.getValue("id").jsonPrimitive.content)
        assertEquals(2, excluded.getValue("transports").jsonArray.size)
        // A resident key is what makes the later login usernameless, so it has to
        // survive the hand-over intact.
        assertEquals(
            "required",
            handedOver.getValue("authenticatorSelection").jsonObject.getValue("residentKey").jsonPrimitive.content,
        )
        assertEquals("300000", handedOver.getValue("timeout").jsonPrimitive.content)
    }

    @Test
    fun `a ceremony with no options is refused rather than sent to the authenticator`() {
        assertFailsWith<IllegalArgumentException> { ceremony("{}").optionsJson() }
        assertFailsWith<IllegalArgumentException> { ceremony("""{"publicKey":{}}""").optionsJson() }
    }

    @Test
    fun `the authenticator's answer is posted back exactly as it was produced`() {
        val response =
            """
            {"id":"Zm9v-_","rawId":"Zm9v-_","type":"public-key",
             "response":{"clientDataJSON":"eyJ0-XBl","authenticatorData":"SZYN5Y_g","signature":"MEUCIQ-_",
             "userHandle":"AQIDBA"},"clientExtensionResults":{}}
            """.trimIndent()

        val credential = passkeyCredentialOf(response)

        assertEquals("Zm9v-_", credential.getValue("rawId").jsonPrimitive.content)
        val inner = credential.getValue("response").jsonObject
        assertEquals("MEUCIQ-_", inner.getValue("signature").jsonPrimitive.content)
        // The user handle is what makes the login usernameless: it names the
        // account the authenticator just proved ownership of, and a client that
        // dropped it would leave the server nothing to resolve.
        assertEquals("AQIDBA", inner.getValue("userHandle").jsonPrimitive.content)
        assertTrue(credential["clientExtensionResults"] is JsonObject)
    }

    @Test
    fun `an answer that is not a credential object is refused`() {
        assertFailsWith<IllegalArgumentException> { passkeyCredentialOf("\"cancelled\"") }
    }
}
