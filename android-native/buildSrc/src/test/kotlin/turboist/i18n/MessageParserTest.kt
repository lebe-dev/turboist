package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class MessageParserTest {
    @Test
    fun `plain text is one literal`() {
        assertEquals(listOf(MessagePart.Literal("Delete")), MessageParser.parse("Delete"))
    }

    @Test
    fun `an argument splits the surrounding text`() {
        assertEquals(
            listOf(
                MessagePart.Literal("Unpin "),
                MessagePart.Argument("name"),
                MessagePart.Literal("!"),
            ),
            MessageParser.parse("Unpin {name}!"),
        )
    }

    @Test
    fun `an apostrophe is text, not an escape`() {
        assertEquals(
            listOf(
                MessagePart.Literal("You'll keep "),
                MessagePart.Argument("count"),
                MessagePart.Literal(" of them"),
            ),
            MessageParser.parse("You'll keep {count} of them"),
        )
    }

    @Test
    fun `a plural keeps its branches and the text around it`() {
        val parts = MessageParser.parse("{count, plural, one {# task created} other {# tasks created}} from template")

        assertEquals(2, parts.size)
        val plural = parts[0] as MessagePart.Plural
        assertEquals("count", plural.name)
        assertEquals(listOf("one", "other"), plural.branches.map { it.selector })
        assertEquals(
            listOf(MessagePart.PluralNumber("count"), MessagePart.Literal(" task created")),
            plural.branches[0].parts,
        )
        assertEquals(MessagePart.Literal(" from template"), parts[1])
    }

    @Test
    fun `an exact branch is parsed alongside the categories`() {
        val plural = MessageParser.parse(
            "{count, plural, =0 {No subtasks} one {# subtask} other {# subtasks}}",
        ).single() as MessagePart.Plural

        assertEquals(listOf("=0", "one", "other"), plural.branches.map { it.selector })
    }

    @Test
    fun `an argument inside a branch is parsed`() {
        val plural = MessageParser.parse(
            "{count, plural, one {# of {total}} other {# of {total}}}",
        ).single() as MessagePart.Plural

        assertEquals(
            listOf(
                MessagePart.PluralNumber("count"),
                MessagePart.Literal(" of "),
                MessagePart.Argument("total"),
            ),
            plural.branches[0].parts,
        )
    }

    @Test
    fun `an unsupported argument type is rejected`() {
        assertFailsWith<IllegalArgumentException> {
            MessageParser.parse("{gender, select, male {He} other {They}}")
        }
    }

    @Test
    fun `unbalanced braces are rejected`() {
        assertFailsWith<IllegalArgumentException> { MessageParser.parse("Hello {name") }
        assertFailsWith<IllegalArgumentException> { MessageParser.parse("Hello name}") }
    }

    @Test
    fun `a hash outside a plural is ordinary text`() {
        assertEquals(listOf(MessagePart.Literal("Issue #12")), MessageParser.parse("Issue #12"))
    }
}
