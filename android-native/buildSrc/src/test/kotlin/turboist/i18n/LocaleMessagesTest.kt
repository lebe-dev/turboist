package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class LocaleMessagesTest {
    @Test
    fun `nested objects flatten onto underscore separated names`() {
        val flat = LocaleMessages.flatten(
            """{"page": {"task": {"title": "Task"}}, "app": {"name": "Turboist"}}""",
        )

        assertEquals(mapOf("app_name" to "Turboist", "page_task_title" to "Task"), flat)
    }

    @Test
    fun `names come out sorted so the generated file is stable`() {
        val flat = LocaleMessages.flatten("""{"z": "last", "a": "first", "m": {"b": "middle"}}""")

        assertEquals(listOf("a", "m_b", "z"), flat.keys.toList())
    }

    @Test
    fun `two keys flattening onto one name fail instead of overwriting`() {
        val failure = assertFailsWith<IllegalArgumentException> {
            LocaleMessages.flatten("""{"a": {"b_c": "one"}, "a_b": {"c": "two"}}""")
        }

        assertTrue(failure.message!!.contains("a_b_c"), failure.message)
    }

    @Test
    fun `a non string leaf is rejected rather than guessed at`() {
        assertFailsWith<IllegalArgumentException> {
            LocaleMessages.flatten("""{"limit": 5}""")
        }
        assertFailsWith<IllegalArgumentException> {
            LocaleMessages.flatten("""{"items": ["a"]}""")
        }
    }

    @Test
    fun `a key that cannot be an Android resource name is rejected`() {
        assertFailsWith<IllegalArgumentException> {
            LocaleMessages.flatten("""{"2fa": {"title": "Two factor"}}""")
        }
        assertFailsWith<IllegalArgumentException> {
            LocaleMessages.flatten("""{"next-week": "Next week"}""")
        }
    }
}
