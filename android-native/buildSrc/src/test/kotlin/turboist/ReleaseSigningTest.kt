package turboist

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ReleaseSigningTest {
    private val complete = mapOf(
        ReleaseSigning.STORE_FILE to "/secrets/turboist.jks",
        ReleaseSigning.STORE_PASSWORD to "store-secret",
        ReleaseSigning.KEY_ALIAS to "turboist",
        ReleaseSigning.KEY_PASSWORD to "key-secret",
    )

    private fun from(values: Map<String, String>) = ReleaseSigning.from { values[it] }

    @Test
    fun `a machine that supplies nothing gets no signing config`() {
        assertNull(from(emptyMap()))
    }

    @Test
    fun `blank values count as absent`() {
        assertNull(from(complete.mapValues { "   " }))
    }

    @Test
    fun `a complete set is accepted and trimmed`() {
        val credentials = from(complete.mapValues { (_, value) -> "  $value \n" })
        assertEquals("/secrets/turboist.jks", credentials?.storeFile)
        assertEquals("store-secret", credentials?.storePassword)
        assertEquals("turboist", credentials?.keyAlias)
        assertEquals("key-secret", credentials?.keyPassword)
    }

    @Test
    fun `a half-configured machine is refused instead of silently building unsigned`() {
        val failure = assertFailsWith<IllegalStateException> {
            from(complete - ReleaseSigning.KEY_PASSWORD - ReleaseSigning.KEY_ALIAS)
        }
        assertTrue(failure.message!!.contains(ReleaseSigning.KEY_ALIAS), failure.message)
        assertTrue(failure.message!!.contains(ReleaseSigning.KEY_PASSWORD), failure.message)
        // The names that were supplied stay out of the message: one of them is a
        // password, and an error text is the least private place in a build log.
        assertTrue(!failure.message!!.contains("store-secret"), failure.message)
    }

    @Test
    fun `the four names are the ones a build machine is asked for`() {
        assertEquals(
            listOf(
                "TURBOIST_ANDROID_KEYSTORE",
                "TURBOIST_ANDROID_KEYSTORE_PASSWORD",
                "TURBOIST_ANDROID_KEY_ALIAS",
                "TURBOIST_ANDROID_KEY_PASSWORD",
            ),
            ReleaseSigning.NAMES,
        )
    }
}
