package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals

class StringResourceXmlTest {
    @Test
    fun `resources are written sorted by name`() {
        val xml = StringResourceXml.render(
            listOf(
                AndroidResource.Text("zeta", "Z"),
                AndroidResource.Text("alpha", "A"),
            ),
            note = "generated",
        )

        assertEquals(
            listOf("""    <string name="alpha">A</string>""", """    <string name="zeta">Z</string>"""),
            xml.lines().filter { it.contains("<string") },
        )
    }

    @Test
    fun `a plural is written as items keyed by quantity`() {
        val xml = StringResourceXml.render(
            listOf(AndroidResource.Plural("created", listOf("one" to "%1\$s task", "other" to "%1\$s tasks"))),
            note = "generated",
        )

        assertEquals(
            """
            <plurals name="created">
                <item quantity="one">%1${'$'}s task</item>
                <item quantity="other">%1${'$'}s tasks</item>
            </plurals>
            """.trimIndent(),
            xml.lines().filter { it.contains("<plurals") || it.contains("<item") || it.contains("</plurals") }
                .joinToString("\n") { it.removePrefix("    ") },
        )
    }

    @Test
    fun `declared names cover both strings and plurals`() {
        val names = StringResourceXml.declaredNames(
            """
            <?xml version="1.0" encoding="utf-8"?>
            <resources>
                <string name="native_connecting">Connecting</string>
                <plurals name="native_task_count"><item quantity="other">%1${'$'}s</item></plurals>
                <color name="brand">#fff</color>
            </resources>
            """.trimIndent(),
        )

        assertEquals(setOf("native_connecting", "native_task_count"), names)
    }
}
