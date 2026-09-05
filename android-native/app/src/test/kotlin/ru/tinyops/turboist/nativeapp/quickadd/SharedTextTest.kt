package ru.tinyops.turboist.nativeapp.quickadd

import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * What another app's share becomes.
 *
 * The rule has to hold for the three shapes that actually arrive: a bare
 * sentence, a page (a name plus its address), and a wall of text somebody
 * highlighted. In all three, one line becomes the title and everything else is
 * kept in the description — nothing shared is ever dropped, and the title never
 * becomes a paragraph no list can draw.
 */
class SharedTextTest {
    private fun shared(
        text: String? = null,
        subject: String? = null,
    ) = quickAddRequest(SharedText(subject = subject, text = text))

    @Test
    fun `a single sentence becomes the whole task`() {
        val request = shared(text = "Renew the passport")

        assertEquals(QuickAddRequest(title = "Renew the passport"), request)
    }

    @Test
    fun `a page shares as its name over its address`() {
        val request = shared(subject = "Kotlin coroutines guide", text = "https://example.test/guide")

        assertEquals("Kotlin coroutines guide", request.title)
        assertEquals("https://example.test/guide", request.description)
    }

    @Test
    fun `several lines put the first one in the title and keep the rest`() {
        val request = shared(text = "Call the dentist\nabout the appointment\non Tuesday")

        assertEquals("Call the dentist", request.title)
        assertEquals("about the appointment\non Tuesday", request.description)
    }

    @Test
    fun `leading blank lines are not mistaken for the title`() {
        val request = shared(text = "\n\n   \nBook the flights\nfrom Berlin")

        assertEquals("Book the flights", request.title)
        assertEquals("from Berlin", request.description)
    }

    /**
     * The sheet reads one line as one task, so a share must never arrive holding
     * two. Whitespace inside a subject is flattened for the same reason.
     */
    @Test
    fun `a subject spanning lines is flattened into one title`() {
        val request = shared(subject = "Quarterly  review\nand plan")

        assertEquals("Quarterly review and plan", request.title)
        assertTrue(request.description.isEmpty())
    }

    @Test
    fun `a shared paragraph is cut at a word, and what was cut is kept`() {
        val word = "word"
        val paragraph = List(60) { word }.joinToString(" ")

        val request = shared(text = paragraph)

        assertTrue(request.title.length <= 120, "the title was left too long to draw: ${request.title.length}")
        assertTrue(request.title.endsWith(word), "the title was cut mid-word: ${request.title}")
        assertEquals(paragraph, "${request.title} ${request.description}")
    }

    @Test
    fun `a single word longer than a title is cut where the limit falls`() {
        val request = shared(text = "x".repeat(200))

        assertEquals(120, request.title.length)
        assertEquals(80, request.description.length)
    }

    @Test
    fun `an empty share opens an empty sheet rather than a task called nothing`() {
        assertEquals(QuickAddRequest(), shared(text = "   \n  "))
        assertEquals(QuickAddRequest(), shared())
    }
}
