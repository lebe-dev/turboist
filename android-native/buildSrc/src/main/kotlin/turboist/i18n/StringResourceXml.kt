package turboist.i18n

import org.xml.sax.InputSource
import java.io.StringReader
import javax.xml.parsers.DocumentBuilderFactory

/**
 * Renders and reads the `strings.xml` shape.
 *
 * Output is deterministic — resources sorted by name, one fixed layout, no
 * timestamps — so regenerating without a wording change leaves the file
 * byte-identical and the build stays incremental.
 */
object StringResourceXml {
    private const val HEADER = """<?xml version="1.0" encoding="utf-8"?>"""

    /**
     * Writes the resource list.
     *
     * [note] is placed in a comment at the top of the file, where it tells the
     * next reader that editing this file is pointless.
     */
    fun render(resources: List<AndroidResource>, note: String): String {
        val out = StringBuilder()
        out.append(HEADER).append('\n')
        out.append("<!--\n")
        note.lines().forEach { line -> out.append("  ").append(line).append('\n') }
        out.append("-->\n")
        out.append("<resources>\n")
        for (resource in resources.sortedBy { it.name }) {
            when (resource) {
                is AndroidResource.Text ->
                    out.append("    <string name=\"").append(resource.name).append("\">")
                        .append(resource.value).append("</string>\n")

                is AndroidResource.Plural -> {
                    out.append("    <plurals name=\"").append(resource.name).append("\">\n")
                    for ((quantity, value) in resource.items) {
                        out.append("        <item quantity=\"").append(quantity).append("\">")
                            .append(value).append("</item>\n")
                    }
                    out.append("    </plurals>\n")
                }
            }
        }
        out.append("</resources>\n")
        return out.toString()
    }

    /**
     * Collects the `<string>` and `<plurals>` names a hand-written resource file
     * declares, so generated wording can be checked against it.
     */
    fun declaredNames(xml: String): Set<String> {
        val factory = DocumentBuilderFactory.newInstance().apply {
            isNamespaceAware = false
            // Resource files are local build inputs, but there is no reason for
            // the parser to be able to reach out of the file it was handed.
            runCatching { setFeature("http://apache.org/xml/features/disallow-doctype-decl", true) }
        }
        val document = factory.newDocumentBuilder().parse(InputSource(StringReader(xml)))
        val names = LinkedHashSet<String>()
        for (tag in listOf("string", "plurals")) {
            val nodes = document.getElementsByTagName(tag)
            for (index in 0 until nodes.length) {
                val name = nodes.item(index).attributes?.getNamedItem("name")?.nodeValue ?: continue
                names += name
            }
        }
        return names
    }
}
