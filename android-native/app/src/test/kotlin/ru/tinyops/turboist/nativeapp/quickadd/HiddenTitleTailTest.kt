package ru.tinyops.turboist.nativeapp.quickadd

import androidx.compose.ui.text.AnnotatedString
import org.junit.Test
import ru.tinyops.turboist.nativeapp.quickadd.ui.HiddenTitleTail
import kotlin.test.assertEquals

/** What the title field draws while a title is being typed. */
class HiddenTitleTailTest {
    private fun drawn(text: String): String = HiddenTitleTail.filter(AnnotatedString(text)).text.text

    @Test
    fun `a dot at the end of a line is not drawn`() {
        assertEquals("Buy milk", drawn("Buy milk."))
        assertEquals("Buy milk", drawn("Buy milk..."))
        assertEquals("Buy milk\nCall", drawn("Buy milk.\nCall."))
    }

    @Test
    fun `a dot comes back as soon as something follows it`() {
        assertEquals("v2.5", drawn("v2.5"))
        assertEquals("One. Two", drawn("One. Two"))
    }

    @Test
    fun `a space stays so the cursor can move past it`() {
        assertEquals("Buy ", drawn("Buy "))
        assertEquals("Buy ", drawn("Buy. "))
    }

    @Test
    fun `the cursor maps through the hidden dots in both directions`() {
        val mapping = HiddenTitleTail.filter(AnnotatedString("a.\nb.")).offsetMapping

        assertEquals(listOf(0, 1, 1, 2, 3, 3), (0..5).map(mapping::originalToTransformed))
        assertEquals(listOf(0, 2, 3, 5), (0..3).map(mapping::transformedToOriginal))
    }
}
