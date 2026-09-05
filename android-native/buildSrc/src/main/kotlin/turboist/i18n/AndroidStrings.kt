package turboist.i18n

/** A resource ready to be written into `strings.xml`. */
sealed interface AndroidResource {
    val name: String

    /** A `<string>` entry. */
    data class Text(override val name: String, val value: String) : AndroidResource

    /** A `<plurals>` entry; [items] maps an Android quantity onto its wording. */
    data class Plural(override val name: String, val items: List<Pair<String, String>>) : AndroidResource
}

/**
 * Converts parsed locale messages into Android string resources.
 *
 * ### Placeholder mapping
 *
 * The shared locale files name their placeholders (`{count}`, `{name}`), while
 * Android formats by position. Every distinct name in a message therefore gets a
 * position, numbered from 1 in the order the name first appears, and is written
 * as `%N$s`. Repeating a name reuses its position, so `{name} … {name}` still
 * takes a single argument.
 *
 * Positions are assigned from the English message and reused verbatim for every
 * translation, so one call site passes its arguments in one order whatever the
 * device language is. A placeholder that appears only in a translation is
 * appended after the English ones rather than shifting them.
 *
 * Every placeholder becomes `%s` and never `%d`: the locale files carry no type
 * information, and `%s` renders a number the same way while never throwing on a
 * value that turns out to be text.
 *
 * A literal `%` is doubled to `%%` only in messages that take arguments. In a
 * message without arguments Android never runs the formatter, so doubling there
 * would put a stray `%` on screen.
 *
 * ### Plural mapping
 *
 * A message whose body is a `{name, plural, …}` choice becomes `<plurals>`, one
 * `<item quantity="…">` per branch, with any wording surrounding the choice
 * folded into every item (Android has no way to express "this part varies and
 * that part does not"). `#` becomes the position of the plural's own argument,
 * which is why the count is passed twice at the call site: once to choose the
 * branch and once to be formatted into it.
 *
 * Exact-match branches such as `=0` are dropped. Android picks an item by the
 * language's plural category alone and cannot test a number for equality, so an
 * exact branch would never be selected; keeping it would only suggest otherwise.
 *
 * ### Escaping
 *
 * Text is escaped for both XML (`&`, `<`, `>`) and for Android's own string
 * reader (`\`, `'`, `"`, newline, tab, a leading `@` or `?`). A leading or
 * trailing space is written as its unicode escape, which Android keeps where a
 * bare space would be trimmed.
 */
object AndroidStrings {
    private val ANDROID_QUANTITIES = setOf("zero", "one", "two", "few", "many", "other")

    /** The argument names of a message, in the order their positions are assigned. */
    fun argumentOrder(parts: List<MessagePart>): List<String> {
        val order = LinkedHashSet<String>()
        fun walk(nodes: List<MessagePart>) {
            for (part in nodes) {
                when (part) {
                    is MessagePart.Literal -> Unit
                    is MessagePart.Argument -> order += part.name
                    is MessagePart.PluralNumber -> order += part.name
                    is MessagePart.Plural -> {
                        order += part.name
                        part.branches.forEach { walk(it.parts) }
                    }
                }
            }
        }
        walk(parts)
        return order.toList()
    }

    /**
     * Converts one message into a resource.
     *
     * [sourceArguments] is the argument order of the English message; pass the
     * message's own order when converting English itself.
     */
    fun toResource(name: String, message: String, sourceArguments: List<String>): AndroidResource {
        val parts = MessageParser.parse(message)
        val positions = positionsOf(parts, sourceArguments)
        val pluralCount = parts.count { it is MessagePart.Plural }
        require(pluralCount <= 1) { "message '$name' has more than one plural choice" }
        if (pluralCount == 0) return AndroidResource.Text(name, render(parts, positions))

        val items = mutableListOf<Pair<String, String>>()
        val plural = parts.first { it is MessagePart.Plural } as MessagePart.Plural
        for (branch in plural.branches) {
            if (branch.selector !in ANDROID_QUANTITIES) continue
            val expanded = parts.flatMap { part ->
                if (part === plural) branch.parts else listOf(part)
            }
            items += branch.selector to render(expanded, positions)
        }
        require(items.any { it.first == "other" }) {
            "plural '$name' has no 'other' branch, which Android requires"
        }
        return AndroidResource.Plural(name, items)
    }

    private fun positionsOf(parts: List<MessagePart>, sourceArguments: List<String>): Map<String, Int> {
        val names = LinkedHashSet(sourceArguments)
        names += argumentOrder(parts)
        return names.withIndex().associate { (index, argument) -> argument to index + 1 }
    }

    private fun render(parts: List<MessagePart>, positions: Map<String, Int>): String {
        val formatted = parts.any { it !is MessagePart.Literal }
        val out = StringBuilder()
        for (part in parts) {
            when (part) {
                is MessagePart.Literal -> out.append(escape(part.text, doubleFormatPercent = formatted))
                is MessagePart.Argument -> out.append(reference(part.name, positions))
                is MessagePart.PluralNumber -> out.append(reference(part.name, positions))
                is MessagePart.Plural -> throw IllegalArgumentException("nested plural choices are not supported")
            }
        }
        return withEdgeSpacesEscaped(out.toString())
    }

    private fun reference(name: String, positions: Map<String, Int>): String {
        val position = positions[name] ?: throw IllegalArgumentException("argument '$name' has no position")
        return "%$position\$s"
    }

    private fun escape(text: String, doubleFormatPercent: Boolean): String {
        val out = StringBuilder(text.length)
        for (char in text) {
            when (char) {
                '\\' -> out.append("\\\\")
                '\'' -> out.append("\\'")
                '"' -> out.append("\\\"")
                '&' -> out.append("&amp;")
                '<' -> out.append("&lt;")
                '>' -> out.append("&gt;")
                '\n' -> out.append("\\n")
                '\t' -> out.append("\\t")
                '%' -> out.append(if (doubleFormatPercent) "%%" else "%")
                else -> out.append(char)
            }
        }
        return out.toString()
    }

    /**
     * Protects the edges of a value Android would otherwise reshape: a leading
     * `@` or `?` reads as a resource reference, and edge spaces are trimmed.
     */
    private fun withEdgeSpacesEscaped(value: String): String {
        var result = value
        if (result.startsWith("@") || result.startsWith("?")) result = "\\" + result
        if (result.startsWith(" ")) result = "\\u0020" + result.substring(1)
        if (result.endsWith(" ")) {
            result = result.substring(0, result.length - 1) + "\\u0020"
        }
        return result
    }
}
