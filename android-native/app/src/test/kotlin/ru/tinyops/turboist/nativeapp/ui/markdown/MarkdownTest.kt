package ru.tinyops.turboist.nativeapp.ui.markdown

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * The description subset, held to the same reading the web client gives it.
 *
 * The two clients show one another's text, so a description typed on a phone and
 * read in a browser has to mean the same thing in both. Every case below is one
 * the browser already answers this way.
 */
class MarkdownTest {
    private fun plain(segments: List<InlineSegment>): String =
        segments.joinToString("") { segment ->
            when (segment) {
                is InlineSegment.Text -> segment.value
                is InlineSegment.Code -> segment.value
                is InlineSegment.Link -> segment.text
                is InlineSegment.Bold -> plain(segment.segments)
                is InlineSegment.Italic -> plain(segment.segments)
            }
        }

    @Test
    fun `plain prose is one paragraph and nothing else`() {
        val blocks = Markdown.blocks("just a sentence")

        assertEquals(1, blocks.size)
        assertEquals("just a sentence", plain((blocks.single() as MarkdownBlock.Paragraph).segments))
        assertFalse(Markdown.hasContent("just a sentence"))
    }

    @Test
    fun `a blank line ends a paragraph`() {
        val blocks = Markdown.blocks("first\n\nsecond")

        assertEquals(2, blocks.size)
        assertTrue(blocks.all { it is MarkdownBlock.Paragraph })
    }

    @Test
    fun `a heading carries its level`() {
        val blocks = Markdown.blocks("### deep\ntext")

        val heading = blocks.first() as MarkdownBlock.Heading
        assertEquals(3, heading.level)
        assertEquals("deep", plain(heading.segments))
    }

    @Test
    fun `bullets and numbers are separate lists`() {
        val blocks = Markdown.blocks("- one\n- two\n1. three")

        val bulleted = blocks[0] as MarkdownBlock.Items
        val numbered = blocks[1] as MarkdownBlock.Items
        assertFalse(bulleted.ordered)
        assertEquals(listOf("one", "two"), bulleted.items.map(::plain))
        assertTrue(numbered.ordered)
        assertEquals(listOf("three"), numbered.items.map(::plain))
    }

    @Test
    fun `bold, italic and code are told apart`() {
        val segments = Markdown.inline("**b** _i_ `c`")

        assertTrue(segments.any { it is InlineSegment.Bold })
        assertTrue(segments.any { it is InlineSegment.Italic })
        assertTrue(segments.any { it is InlineSegment.Code })
    }

    @Test
    fun `a double marker is bold rather than an empty italic`() {
        val segments = Markdown.inline("**loud**")

        assertEquals(1, segments.size)
        assertTrue(segments.single() is InlineSegment.Bold)
    }

    @Test
    fun `a marker with no partner stays as the character that was typed`() {
        assertEquals("2 * 3 = 6", plain(Markdown.inline("2 * 3 = 6")))
        assertEquals("a_b", plain(Markdown.inline("a_b")))
    }

    @Test
    fun `a link is a link only when its target can be opened safely`() {
        val safe = Markdown.inline("see [docs](https://example.com/x)")
        assertEquals("https://example.com/x", (safe.last() as InlineSegment.Link).href)

        // A scheme that could act on the device on its own is left as text: a
        // description arrives from the server and may have been written anywhere.
        val unsafe = Markdown.inline("[run](javascript:alert(1))")
        assertTrue(unsafe.none { it is InlineSegment.Link })
        assertEquals("[run](javascript:alert(1))", plain(unsafe))
    }

    @Test
    fun `a link makes text worth rendering even with no other markup`() {
        assertTrue(Markdown.hasLink("[a](/today)"))
        assertTrue(Markdown.hasContent("[a](/today)"))
        assertFalse(Markdown.hasLink("[a](javascript:x)"))
    }

    @Test
    fun `markup anywhere in the text counts as content`() {
        assertTrue(Markdown.hasContent("# heading"))
        assertTrue(Markdown.hasContent("a **bold** word"))
        assertTrue(Markdown.hasContent("- item"))
        assertTrue(Markdown.hasContent("`code`"))
        assertFalse(Markdown.hasContent(""))
    }

    @Test
    fun `windows line endings do not leave stray blocks`() {
        val blocks = Markdown.blocks("first\r\n\r\nsecond")

        assertEquals(2, blocks.size)
    }

    @Test
    fun `nested emphasis keeps the inner run`() {
        val bold = Markdown.inline("**very _sure_**").single() as InlineSegment.Bold

        assertTrue(bold.segments.any { it is InlineSegment.Italic })
        assertEquals("very sure", plain(listOf(bold)))
    }
}
