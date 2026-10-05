package turboist.i18n

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class LocaleStringGeneratorTest {
    private val english = """{"nav": {"today": "Today", "week": "Week"}}"""
    private val russian = """{"nav": {"today": "Сегодня", "week": "Неделя"}}"""

    private fun generate(
        source: String = english,
        translations: List<LocaleStringGenerator.LocaleFile> =
            listOf(LocaleStringGenerator.LocaleFile("ru", russian)),
        handWritten: Set<String> = emptySet(),
    ) = LocaleStringGenerator.generate(
        source = LocaleStringGenerator.LocaleFile("en", source),
        translations = translations,
        handWrittenNames = handWritten,
    )

    @Test
    fun `the source locale lands in values and a translation in its qualified folder`() {
        val files = generate()

        assertEquals(listOf("values/strings.xml", "values-ru/strings.xml"), files.keys.toList())
        assertTrue(files.getValue("values/strings.xml").contains("""<string name="nav_today">Today</string>"""))
        assertTrue(files.getValue("values-ru/strings.xml").contains("""<string name="nav_today">Сегодня</string>"""))
    }

    @Test
    fun `a key the translation lacks is left out so Android falls back to the source`() {
        val partial = """{"nav": {"today": "Сегодня"}}"""

        val files = generate(translations = listOf(LocaleStringGenerator.LocaleFile("ru", partial)))

        val translated = files.getValue("values-ru/strings.xml")
        assertTrue(translated.contains("nav_today"))
        assertFalse(translated.contains("nav_week"))
    }

    @Test
    fun `a key only the translation has is dropped`() {
        val extra = """{"nav": {"today": "Сегодня", "week": "Неделя", "ghost": "Призрак"}}"""

        val files = generate(translations = listOf(LocaleStringGenerator.LocaleFile("ru", extra)))

        assertFalse(files.getValue("values-ru/strings.xml").contains("nav_ghost"))
    }

    @Test
    fun `a name a hand written resource already owns fails the build`() {
        val failure = assertFailsWith<IllegalArgumentException> {
            generate(handWritten = setOf("nav_week", "native_connecting"))
        }

        assertTrue(failure.message!!.contains("nav_week"), failure.message)
        assertFalse(failure.message!!.contains("native_connecting"), failure.message)
    }

    @Test
    fun `unrelated hand written names are left alone`() {
        generate(handWritten = setOf("app_name_native", "native_dest_task"))
    }

    @Test
    fun `output is byte identical when nothing changed`() {
        assertEquals(generate(), generate())
    }

    @Test
    fun `a failing message names the key it came from`() {
        val broken = """{"nav": {"today": "{gender, select, other {x}}", "week": "Week"}}"""

        val failure = assertFailsWith<IllegalArgumentException> { generate(source = broken) }

        assertTrue(failure.message!!.contains("nav_today"), failure.message)
    }
}
