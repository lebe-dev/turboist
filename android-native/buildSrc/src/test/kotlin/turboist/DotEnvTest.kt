package turboist

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class DotEnvTest {
    @Test
    fun `reads plain, exported and quoted assignments`() {
        val parsed =
            DotEnv.parse(
                """
                # a comment
                SENTRY_DSN=https://key@sentry.example/1

                export SENTRY_ENVIRONMENT=production
                DOUBLE="with spaces"
                SINGLE='a # b'
                """.trimIndent(),
            )

        assertEquals("https://key@sentry.example/1", parsed["SENTRY_DSN"])
        assertEquals("production", parsed["SENTRY_ENVIRONMENT"])
        assertEquals("with spaces", parsed["DOUBLE"])
        assertEquals("a # b", parsed["SINGLE"])
    }

    @Test
    fun `a hash is a comment only after whitespace`() {
        val parsed = DotEnv.parse("A=value # note\nB=abc#def")

        assertEquals("value", parsed["A"])
        assertEquals("abc#def", parsed["B"])
    }

    @Test
    fun `lines that are not assignments are ignored`() {
        assertEquals(emptyMap<String, String>(), DotEnv.parse("not an assignment\n=value\n1BAD=x"))
    }

    @Test
    fun `the environment wins and the file fills in what it leaves out`() {
        val lookup =
            DotEnv.lookup(
                environment = mapOf("BOTH" to "env", "EMPTY" to " ")::get,
                file = mapOf("BOTH" to "file", "EMPTY" to "file", "ONLY_FILE" to "file", "BLANK" to ""),
            )

        assertEquals("env", lookup("BOTH"))
        assertEquals("file", lookup("EMPTY"))
        assertEquals("file", lookup("ONLY_FILE"))
        assertNull(lookup("BLANK"))
        assertNull(lookup("MISSING"))
    }
}
