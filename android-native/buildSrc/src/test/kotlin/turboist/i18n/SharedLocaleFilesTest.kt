package turboist.i18n

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Runs the converter over the locale files the product actually ships.
 *
 * The unit tests above pin the rules; this one catches the case those rules were
 * never written for — a new phrase using syntax the converter cannot express —
 * at the moment the phrase is added rather than when someone opens the screen.
 */
class SharedLocaleFilesTest {
    private val localesDirectory = File("../../frontend/locales")

    private fun read(code: String): String {
        val file = File(localesDirectory, "$code.json")
        assertTrue(file.isFile, "locale file not found: ${file.absolutePath}")
        return file.readText()
    }

    @Test
    fun `every shipped message converts into a resource`() {
        val files = LocaleStringGenerator.generate(
            source = LocaleStringGenerator.LocaleFile("en", read("en")),
            translations = listOf(LocaleStringGenerator.LocaleFile("ru", read("ru"))),
            handWrittenNames = emptySet(),
        )

        assertEquals(listOf("values/strings.xml", "values-ru/strings.xml"), files.keys.toList())
        files.forEach { (path, content) ->
            assertTrue(content.contains("<resources>"), path)
            assertTrue(content.trimEnd().endsWith("</resources>"), path)
        }
    }

    @Test
    fun `a shipped phrase reaches both locale files`() {
        val files = LocaleStringGenerator.generate(
            source = LocaleStringGenerator.LocaleFile("en", read("en")),
            translations = listOf(LocaleStringGenerator.LocaleFile("ru", read("ru"))),
            handWrittenNames = emptySet(),
        )

        assertTrue(
            files.getValue("values/strings.xml").contains("""<string name="nav_today">Today</string>"""),
            "the English navigation wording is missing",
        )
        assertTrue(
            files.getValue("values-ru/strings.xml").contains("""<string name="nav_today">"""),
            "the Russian navigation wording is missing",
        )
    }
}
