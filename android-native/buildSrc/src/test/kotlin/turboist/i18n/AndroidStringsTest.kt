package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class AndroidStringsTest {
    private fun text(name: String, message: String, source: List<String> = emptyList()): String {
        val order = source.ifEmpty { AndroidStrings.argumentOrder(MessageParser.parse(message)) }
        return (AndroidStrings.toResource(name, message, order) as AndroidResource.Text).value
    }

    @Test
    fun `named placeholders become positional format arguments`() {
        assertEquals("Add to %1\$s", text("add", "Add to {section}"))
        assertEquals("%1\$s of %2\$s", text("ratio", "{used} of {limit}"))
    }

    @Test
    fun `a repeated placeholder reuses its position`() {
        assertEquals("%1\$s and %1\$s again", text("twice", "{name} and {name} again"))
    }

    @Test
    fun `positions follow the source locale so call sites stay stable`() {
        val english = "{used} of {limit}"
        val order = AndroidStrings.argumentOrder(MessageParser.parse(english))

        // The translation reverses the wording; the positions must not follow it.
        assertEquals("из %2\$s занято %1\$s", text("ratio", "из {limit} занято {used}", order))
    }

    @Test
    fun `a placeholder only the translation has is appended after the source ones`() {
        val order = AndroidStrings.argumentOrder(MessageParser.parse("{used} of {limit}"))

        assertEquals("%3\$s", text("extra", "{surprise}", order))
    }

    @Test
    fun `xml and Android specials are escaped`() {
        assertEquals("Backup &amp; Restore", text("backup", "Backup & Restore"))
        assertEquals("You\\'ll stay", text("stay", "You'll stay"))
        assertEquals("Delete \\\"a\\\"?", text("delete", """Delete "a"?"""))
        assertEquals("&lt;b&gt;", text("bold", "<b>"))
        assertEquals("one\\ntwo", text("lines", "one\ntwo"))
        assertEquals("back\\\\slash", text("slash", """back\slash"""))
    }

    @Test
    fun `a leading resource sigil is escaped so it is not read as a reference`() {
        assertEquals("\\@home", text("context", "@home"))
        assertEquals("\\?", text("what", "?"))
    }

    @Test
    fun `edge spaces survive Android's trimming`() {
        assertEquals("\\u0020lead", text("lead", " lead"))
        assertEquals("trail\\u0020", text("trail", "trail "))
    }

    @Test
    fun `a percent sign is doubled only where the formatter runs`() {
        assertEquals("100% done", text("done", "100% done"))
        assertEquals("%1\$s%% done", text("progress", "{value}% done"))
    }

    @Test
    fun `a plural becomes a plurals resource with the surrounding wording folded in`() {
        val message = "{count, plural, one {# task created} other {# tasks created}} from template"
        val order = AndroidStrings.argumentOrder(MessageParser.parse(message))
        val plural = AndroidStrings.toResource("created", message, order) as AndroidResource.Plural

        assertEquals(
            listOf(
                "one" to "%1\$s task created from template",
                "other" to "%1\$s tasks created from template",
            ),
            plural.items,
        )
    }

    @Test
    fun `russian plural categories are all carried over`() {
        val english = "{count, plural, one {# day left} other {# days left}}"
        val order = AndroidStrings.argumentOrder(MessageParser.parse(english))
        val russian = "{count, plural, one {остался # день} few {осталось # дня} " +
            "many {осталось # дней} other {осталось # дней}}"

        val plural = AndroidStrings.toResource("daysLeft", russian, order) as AndroidResource.Plural

        assertEquals(listOf("one", "few", "many", "other"), plural.items.map { it.first })
    }

    @Test
    fun `an exact branch is dropped because Android cannot select on it`() {
        val message = "{count, plural, =0 {No subtasks} one {# subtask} other {# subtasks}}"
        val order = AndroidStrings.argumentOrder(MessageParser.parse(message))
        val plural = AndroidStrings.toResource("subtasks", message, order) as AndroidResource.Plural

        assertEquals(listOf("one", "other"), plural.items.map { it.first })
    }

    @Test
    fun `a plural without an other branch is rejected`() {
        val message = "{count, plural, one {# task}}"
        val order = AndroidStrings.argumentOrder(MessageParser.parse(message))

        val failure = assertFailsWith<IllegalArgumentException> {
            AndroidStrings.toResource("tasks", message, order)
        }
        assertTrue(failure.message!!.contains("other"), failure.message)
    }
}
