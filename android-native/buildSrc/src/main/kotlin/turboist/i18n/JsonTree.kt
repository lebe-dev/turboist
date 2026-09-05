package turboist.i18n

/** A node of a parsed locale file: either a group of keys or a message. */
sealed interface JsonNode {
    /** A nested group; [entries] keeps the file's own key order. */
    data class Group(val entries: Map<String, JsonNode>) : JsonNode

    /** A message, already unescaped. */
    data class Message(val text: String) : JsonNode
}

/**
 * Reads the shape a locale file is allowed to have: nested objects whose leaves
 * are strings.
 *
 * The build logic runs on Gradle's embedded Kotlin runtime, which is older than
 * the one the app compiles against, so a current JSON library cannot be put on
 * this classpath. Locale files are a narrow, well-known dialect, and reading
 * them here keeps the build logic free of that version coupling.
 *
 * Anything the dialect does not cover — a number, a boolean, an array — is
 * refused with its position instead of being coerced, because a locale file that
 * grew such a value is a wording change nobody has decided how to render.
 */
object JsonTree {
    /** Parses a whole locale file. */
    fun parse(source: String): JsonNode.Group {
        val reader = Reader(source)
        reader.skipWhitespace()
        val root = reader.readGroup()
        reader.skipWhitespace()
        reader.ensure(reader.atEnd(), "trailing content after the top-level object")
        return root
    }

    private class Reader(private val source: String) {
        private var index = 0

        fun atEnd(): Boolean = index >= source.length

        fun skipWhitespace() {
            while (index < source.length && source[index].isWhitespace()) index++
        }

        fun readGroup(): JsonNode.Group {
            expect('{')
            val entries = LinkedHashMap<String, JsonNode>()
            skipWhitespace()
            if (peek() == '}') {
                index++
                return JsonNode.Group(entries)
            }
            while (true) {
                skipWhitespace()
                val key = readString()
                skipWhitespace()
                expect(':')
                skipWhitespace()
                entries[key] = readValue()
                skipWhitespace()
                when (val next = peek()) {
                    ',' -> index++
                    '}' -> {
                        index++
                        return JsonNode.Group(entries)
                    }

                    else -> fail("expected ',' or '}' but found '$next'")
                }
            }
        }

        fun ensure(condition: Boolean, message: String) {
            if (!condition) fail(message)
        }

        private fun readValue(): JsonNode = when (peek()) {
            '{' -> readGroup()
            '"' -> JsonNode.Message(readString())
            else -> fail("a locale value must be a string or a nested object")
        }

        private fun readString(): String {
            expect('"')
            val out = StringBuilder()
            while (true) {
                ensure(index < source.length, "unterminated string")
                when (val char = source[index++]) {
                    '"' -> return out.toString()
                    '\\' -> out.append(readEscape())
                    else -> out.append(char)
                }
            }
        }

        private fun readEscape(): String {
            ensure(index < source.length, "unterminated escape")
            return when (val char = source[index++]) {
                '"', '\\', '/' -> char.toString()
                'b' -> "\b"
                'f' -> "\u000C"
                'n' -> "\n"
                'r' -> "\r"
                't' -> "\t"
                'u' -> {
                    ensure(index + 4 <= source.length, "truncated unicode escape")
                    val code = source.substring(index, index + 4)
                    index += 4
                    val value = code.toIntOrNull(16) ?: fail("'$code' is not a unicode escape")
                    Char(value).toString()
                }

                else -> fail("unknown escape '\\$char'")
            }
        }

        private fun peek(): Char {
            ensure(index < source.length, "unexpected end of file")
            return source[index]
        }

        private fun expect(char: Char) {
            ensure(index < source.length && source[index] == char, "expected '$char'")
            index++
        }

        private fun fail(message: String): Nothing =
            throw IllegalArgumentException("$message (at offset $index)")
    }
}
