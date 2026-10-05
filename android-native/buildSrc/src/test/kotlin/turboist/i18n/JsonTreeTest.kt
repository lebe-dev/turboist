package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class JsonTreeTest {
    @Test
    fun `nested groups and messages are read`() {
        val root = JsonTree.parse("""{"app": {"name": "Turboist"}, "retry": "Retry"}""")

        assertEquals(
            JsonNode.Group(
                mapOf(
                    "app" to JsonNode.Group(mapOf("name" to JsonNode.Message("Turboist"))),
                    "retry" to JsonNode.Message("Retry"),
                ),
            ),
            root,
        )
    }

    @Test
    fun `string escapes are unescaped`() {
        val root = JsonTree.parse("""{"a": "line\nbreak", "b": "quote\"mark", "c": "é", "d": "back\\slash"}""")

        val entries = root.entries.mapValues { (_, node) -> (node as JsonNode.Message).text }
        assertEquals("line\nbreak", entries["a"])
        assertEquals("quote\"mark", entries["b"])
        assertEquals("é", entries["c"])
        assertEquals("""back\slash""", entries["d"])
    }

    @Test
    fun `an empty group is allowed`() {
        assertEquals(JsonNode.Group(emptyMap()), JsonTree.parse("{}"))
    }

    @Test
    fun `a value that is neither a string nor a group is refused`() {
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""{"count": 3}""") }
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""{"list": []}""") }
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""{"flag": true}""") }
    }

    @Test
    fun `malformed input is refused`() {
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""{"a": "b" """) }
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""{"a": "b"} extra""") }
        assertFailsWith<IllegalArgumentException> { JsonTree.parse("""["a"]""") }
    }
}
