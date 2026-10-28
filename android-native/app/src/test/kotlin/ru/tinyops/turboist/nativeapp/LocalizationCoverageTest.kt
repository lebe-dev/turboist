package ru.tinyops.turboist.nativeapp

import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Every word the native client shows exists in every language it ships.
 *
 * Wording shared with the web client is generated from the shared locale files
 * and is complete by construction. The hand-written `native_` strings are not:
 * a string added in English alone falls back to English on a Russian phone, in
 * the middle of an otherwise translated screen, and nothing else catches it —
 * the lint check for it is off because the generated files would trip it.
 *
 * The scan of the Kotlin sources is the other half: a sentence typed straight
 * into a composable never reaches a resource file at all.
 */
class LocalizationCoverageTest {
    private val res = File("src/main/res")
    private val sources = File("src/main/kotlin")

    /** The languages the native strings are translated into, besides English. */
    private val translations = listOf("ru")

    @Test
    fun `every translatable native string has a translation in every shipped language`() {
        val english = strings(File(res, "values/strings_native.xml")).filterValues { it.translatable }
        for (code in translations) {
            val translated = strings(File(res, "values-$code/strings_native.xml"))
            val missing = english.keys - translated.keys
            assertTrue(missing.isEmpty(), "values-$code/strings_native.xml lacks: ${missing.sorted()}")
        }
    }

    @Test
    fun `a translation takes the same arguments as the English it translates`() {
        val english = strings(File(res, "values/strings_native.xml"))
        for (code in translations) {
            for ((name, resource) in strings(File(res, "values-$code/strings_native.xml"))) {
                val source = english[name] ?: continue
                assertEquals(source.arguments, resource.arguments, "$name in values-$code")
            }
        }
    }

    @Test
    fun `no composable shows text typed into the source instead of a resource`() {
        val offenders =
            sources
                .walkTopDown()
                .filter { it.isFile && it.extension == "kt" }
                .flatMap { file ->
                    file.readLines().withIndex().mapNotNull { (index, line) ->
                        if (COMMENT.containsMatchIn(line)) return@mapNotNull null
                        val literal =
                            UI_TEXT.findAll(line).map { it.groupValues[1] }.firstOrNull { readsAsWords(it) }
                                ?: return@mapNotNull null
                        "${file.relativeTo(sources)}:${index + 1}: \"$literal\""
                    }
                }.toList()
        assertTrue(offenders.isEmpty(), "move these into strings_native.xml:\n" + offenders.joinToString("\n"))
    }

    private class StringResource(
        val translatable: Boolean,
        val arguments: Set<String>,
    )

    private fun strings(file: File): Map<String, StringResource> {
        val root = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).documentElement
        val nodes = root.childNodes
        return (0 until nodes.length)
            .map { nodes.item(it) }
            .filterIsInstance<Element>()
            .filter { it.tagName == "string" || it.tagName == "plurals" }
            .associate { element ->
                element.getAttribute("name") to
                    StringResource(
                        translatable = element.getAttribute("translatable") != "false",
                        arguments = ARGUMENT.findAll(element.textContent).map { it.value }.toSet(),
                    )
            }
    }

    /** True when a literal holds words of its own once its interpolations are taken out. */
    private fun readsAsWords(literal: String): Boolean = WORD.containsMatchIn(literal.replace(INTERPOLATION, ""))

    private companion object {
        val ARGUMENT = Regex("""%\d+\$[sd]""")
        val COMMENT = Regex("""^\s*(//|/?\*)""")
        val UI_TEXT =
            Regex(
                """(?:\bText\(\s*|\btext\s*=\s*|contentDescription\s*=\s*|stateDescription\s*=\s*)""" +
                    """"((?:[^"\\]|\\.)*)"""",
            )
        val INTERPOLATION = Regex("""\$\{[^}]*\}|\$\w+""")
        val WORD = Regex("""\p{L}{2,}""")
    }
}
