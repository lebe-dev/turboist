package turboist.i18n

/** One piece of a parsed locale message. */
sealed interface MessagePart {
    /** Text that is shown as written. */
    data class Literal(val text: String) : MessagePart

    /** A `{name}` reference to a value the caller supplies. */
    data class Argument(val name: String) : MessagePart

    /**
     * A `{name, plural, one {...} other {...}}` choice.
     *
     * [branches] keeps the selectors in the order the message wrote them; a
     * selector is either a plural category (`one`, `few`, `many`, `other`, …) or
     * an exact match such as `=0`.
     */
    data class Plural(val name: String, val branches: List<PluralBranch>) : MessagePart

    /** The `#` shorthand for the number a surrounding plural is choosing on. */
    data class PluralNumber(val name: String) : MessagePart
}

/** One selector of a plural message together with the wording it selects. */
data class PluralBranch(val selector: String, val parts: List<MessagePart>)

/**
 * Reads the message syntax the shared locale files use.
 *
 * Only the two forms the wording actually relies on are understood: a bare
 * `{name}` argument, and a `{name, plural, selector {…} …}` choice with `#`
 * standing for the chosen number. Any other argument type is rejected rather
 * than passed through, because silently emitting an unreadable Android string
 * would surface as mojibake in the app instead of as a build failure.
 *
 * Single quotes are ordinary characters here. The shared wording uses them as
 * apostrophes ("You'll stay logged in"), never as the message-syntax escape,
 * and reading them as an escape would swallow half a sentence.
 */
object MessageParser {
    private val PLURAL_CATEGORIES = setOf("zero", "one", "two", "few", "many", "other")
    private val EXACT_SELECTOR = Regex("""=\d+""")

    /** Parses a whole message. */
    fun parse(message: String): List<MessagePart> = parseParts(message, pluralArgument = null)

    private fun parseParts(text: String, pluralArgument: String?): List<MessagePart> {
        val parts = mutableListOf<MessagePart>()
        val literal = StringBuilder()
        var index = 0
        while (index < text.length) {
            val char = text[index]
            when {
                char == '#' && pluralArgument != null -> {
                    flush(literal, parts)
                    parts += MessagePart.PluralNumber(pluralArgument)
                    index++
                }

                char == '{' -> {
                    flush(literal, parts)
                    val end = matchingBrace(text, index)
                    parts += parseArgument(text.substring(index + 1, end))
                    index = end + 1
                }

                char == '}' -> throw IllegalArgumentException("unbalanced '}' in message: $text")

                else -> {
                    literal.append(char)
                    index++
                }
            }
        }
        flush(literal, parts)
        return parts
    }

    private fun flush(literal: StringBuilder, parts: MutableList<MessagePart>) {
        if (literal.isEmpty()) return
        parts += MessagePart.Literal(literal.toString())
        literal.setLength(0)
    }

    private fun parseArgument(inner: String): MessagePart {
        val comma = inner.indexOf(',')
        if (comma < 0) {
            val name = inner.trim()
            require(name.isNotEmpty()) { "empty argument in message" }
            return MessagePart.Argument(name)
        }
        val name = inner.substring(0, comma).trim()
        val rest = inner.substring(comma + 1).trimStart()
        val typeEnd = rest.indexOf(',')
        require(typeEnd >= 0) { "argument '$name' has a type but no body" }
        val type = rest.substring(0, typeEnd).trim()
        require(type == "plural") { "argument '$name' uses the unsupported type '$type'" }
        return MessagePart.Plural(name, parseBranches(name, rest.substring(typeEnd + 1)))
    }

    private fun parseBranches(name: String, body: String): List<PluralBranch> {
        val branches = mutableListOf<PluralBranch>()
        var index = 0
        while (index < body.length) {
            if (body[index].isWhitespace()) {
                index++
                continue
            }
            val selectorEnd = body.indexOfFirst(index) { it == '{' || it.isWhitespace() }
            require(selectorEnd > index) { "plural '$name' has a branch with no selector" }
            val selector = body.substring(index, selectorEnd)
            require(selector in PLURAL_CATEGORIES || EXACT_SELECTOR.matches(selector)) {
                "plural '$name' uses the unknown selector '$selector'"
            }
            val open = body.indexOf('{', selectorEnd)
            require(open >= 0) { "plural '$name' selector '$selector' has no body" }
            val close = matchingBrace(body, open)
            branches += PluralBranch(selector, parseParts(body.substring(open + 1, close), pluralArgument = name))
            index = close + 1
        }
        require(branches.isNotEmpty()) { "plural '$name' has no branches" }
        return branches
    }

    private inline fun String.indexOfFirst(from: Int, predicate: (Char) -> Boolean): Int {
        for (i in from until length) {
            if (predicate(this[i])) return i
        }
        return length
    }

    private fun matchingBrace(text: String, open: Int): Int {
        var depth = 0
        for (index in open until text.length) {
            when (text[index]) {
                '{' -> depth++
                '}' -> {
                    depth--
                    if (depth == 0) return index
                }
            }
        }
        throw IllegalArgumentException("unbalanced '{' in message: $text")
    }
}
