package ru.tinyops.turboist.nativeapp.ui.markdown

/**
 * The subset of Markdown a task description is written in.
 *
 * It is deliberately the same subset the web client renders, parsed by the same
 * rules, because the two clients read one another's text: a description typed on
 * a phone is read in a browser the next morning and must look like what its
 * author meant. Anything outside the subset stays visible as the characters that
 * were typed rather than disappearing — a description is the user's own text,
 * and swallowing part of it is worse than showing a stray asterisk.
 *
 * Links are limited to schemes that cannot act on the device on their own. A
 * description arrives from the server and may have been written anywhere, so a
 * link is only turned into something tappable when it is plainly a web address,
 * a mail address, or a path inside the product.
 */
object Markdown {
    private val LINK = Regex("""\[([^\]]+)\]\(([^)\s]+)\)""")
    private val SAFE_SCHEME = Regex("""^(https?://|mailto:|/|#)""", RegexOption.IGNORE_CASE)
    private val HEADING = Regex("""^(#{1,6})\s+(.*)$""")
    private val UNORDERED_ITEM = Regex("""^[-*+]\s+(.*)$""")
    private val ORDERED_ITEM = Regex("""^\d+\.\s+(.*)$""")

    /**
     * Whether the text carries anything worth rendering as Markdown.
     *
     * Used to decide between showing a description as formatted text and showing
     * it as the plain text it was typed as. Plain prose is left alone: running it
     * through a renderer that changes nothing only costs the user the ability to
     * see their own spacing.
     */
    private val RICH =
        Regex(
            """(^|\n)#{1,6}\s+\S""" +
                """|\*\*[^\s*][^*]*\*\*""" +
                """|(^|\s)\*[^\s*][^*]*\*""" +
                """|(^|\s)_[^\s_][^_]*_""" +
                """|`[^`]+`""" +
                """|(^|\n)([-*+]|\d+\.)\s+\S""",
        )

    /** True when [input] holds a link this renderer would make tappable. */
    fun hasLink(input: String): Boolean {
        val match = LINK.find(input) ?: return false
        return SAFE_SCHEME.containsMatchIn(match.groupValues[2])
    }

    /** True when [input] holds any markup at all, a link included. */
    fun hasContent(input: String): Boolean {
        if (input.isEmpty()) return false
        return hasLink(input) || RICH.containsMatchIn(input)
    }

    /** The text cut into blocks: headings, paragraphs and lists, in the order written. */
    fun blocks(input: String): List<MarkdownBlock> {
        if (input.isEmpty()) return emptyList()
        val builder = BlockBuilder()
        for (raw in input.replace("\r\n", "\n").replace('\r', '\n').split('\n')) {
            builder.take(raw.trimEnd())
        }
        return builder.finish()
    }

    /**
     * One block of text cut into its runs: bold, italic, code, links and the
     * plain text between them.
     *
     * A marker with no partner, or one wrapped around nothing, is not markup and
     * is kept as the character it is.
     */
    fun inline(input: String): List<InlineSegment> {
        if (input.isEmpty()) return emptyList()
        val segments = mutableListOf<InlineSegment>()
        val text = StringBuilder()
        var i = 0

        fun flush() {
            if (text.isEmpty()) return
            segments += InlineSegment.Text(text.toString())
            text.clear()
        }

        while (i < input.length) {
            val consumed = readCode(input, i) ?: readLink(input, i) ?: readEmphasis(input, i)
            if (consumed == null) {
                text.append(input[i])
                i++
                continue
            }
            flush()
            segments += consumed.segment
            i = consumed.next
        }
        flush()
        return segments
    }

    /** A run of code between single backticks. */
    private fun readCode(
        input: String,
        at: Int,
    ): Consumed? {
        if (input[at] != '`') return null
        val end = input.indexOf('`', at + 1)
        if (end <= at + 1) return null
        return Consumed(InlineSegment.Code(input.substring(at + 1, end)), end + 1)
    }

    /** A `[text](target)` link, when the target is a scheme this renderer will open. */
    private fun readLink(
        input: String,
        at: Int,
    ): Consumed? {
        if (input[at] != '[') return null
        val match = LINK.matchAt(input, at) ?: return null
        val href = match.groupValues[2]
        if (!SAFE_SCHEME.containsMatchIn(href)) return null
        return Consumed(InlineSegment.Link(match.groupValues[1], href), at + match.value.length)
    }

    /**
     * A run wrapped in `*`/`_` for italic or `**`/`__` for bold.
     *
     * The marker has to hug its text on both sides, and a single marker must not
     * be the first half of a double one, which is what keeps `**bold**` from
     * being read as an empty italic followed by stray text.
     */
    private fun readEmphasis(
        input: String,
        at: Int,
    ): Consumed? {
        val marker = input[at]
        if (marker != '*' && marker != '_') return null
        val doubled = at + 1 < input.length && input[at + 1] == marker
        val width = if (doubled) 2 else 1
        val start = at + width
        if (start >= input.length || input[start] == ' ') return null
        val end = input.indexOf(if (doubled) "$marker$marker" else marker.toString(), start)
        if (end <= start) return null
        if (input[end - 1] == ' ') return null
        if (!doubled && end + 1 < input.length && input[end + 1] == marker) return null
        val inner = inline(input.substring(start, end))
        val segment = if (doubled) InlineSegment.Bold(inner) else InlineSegment.Italic(inner)
        return Consumed(segment, end + width)
    }

    private data class Consumed(
        val segment: InlineSegment,
        val next: Int,
    )

    /** Collects lines into blocks, closing whatever the current line ends. */
    private class BlockBuilder {
        private val blocks = mutableListOf<MarkdownBlock>()
        private val paragraph = mutableListOf<String>()
        private var items: MutableList<String>? = null
        private var ordered = false

        fun take(line: String) {
            if (line.isBlank()) {
                closeParagraph()
                closeList()
                return
            }
            val heading = HEADING.find(line)
            if (heading != null) {
                closeParagraph()
                closeList()
                blocks += MarkdownBlock.Heading(heading.groupValues[1].length, inline(heading.groupValues[2]))
                return
            }
            val unordered = UNORDERED_ITEM.find(line)
            val numbered = if (unordered == null) ORDERED_ITEM.find(line) else null
            if (unordered != null || numbered != null) {
                closeParagraph()
                val isOrdered = numbered != null
                if (items != null && ordered != isOrdered) closeList()
                if (items == null) {
                    items = mutableListOf()
                    ordered = isOrdered
                }
                items?.add((unordered ?: numbered)!!.groupValues[1])
                return
            }
            closeList()
            paragraph += line
        }

        fun finish(): List<MarkdownBlock> {
            closeParagraph()
            closeList()
            return blocks.toList()
        }

        private fun closeParagraph() {
            if (paragraph.isEmpty()) return
            blocks += MarkdownBlock.Paragraph(inline(paragraph.joinToString("\n")))
            paragraph.clear()
        }

        private fun closeList() {
            val open = items ?: return
            blocks += MarkdownBlock.Items(ordered, open.map(::inline))
            items = null
        }
    }
}

/** A run of text inside one block. */
sealed interface InlineSegment {
    data class Text(val value: String) : InlineSegment

    data class Bold(val segments: List<InlineSegment>) : InlineSegment

    data class Italic(val segments: List<InlineSegment>) : InlineSegment

    data class Code(val value: String) : InlineSegment

    data class Link(val text: String, val href: String) : InlineSegment
}

/** One block of a description. */
sealed interface MarkdownBlock {
    /** A heading. [level] is 1 to 6, as many as the hashes that introduced it. */
    data class Heading(val level: Int, val segments: List<InlineSegment>) : MarkdownBlock

    data class Paragraph(val segments: List<InlineSegment>) : MarkdownBlock

    /** A list. [ordered] tells a numbered list from a bulleted one. */
    data class Items(val ordered: Boolean, val items: List<List<InlineSegment>>) : MarkdownBlock
}
