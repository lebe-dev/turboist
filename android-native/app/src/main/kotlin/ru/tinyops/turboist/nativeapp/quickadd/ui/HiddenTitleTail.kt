package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.OffsetMapping
import androidx.compose.ui.text.input.TransformedText
import androidx.compose.ui.text.input.VisualTransformation

/**
 * Draws the title field without the dots at the end of each line, as they are typed.
 *
 * They are hidden rather than deleted, because nobody can tell yet whether a dot
 * ends the title: the next keystroke may turn it into "3.5" or "example.com", and
 * those have to come out whole. Deleting text under the keyboard would also pull it
 * out of step with what the keyboard is composing. Spaces stay, since they draw
 * nothing and the cursor moving past one is how the user knows it was typed. The
 * title that is saved has both taken off.
 */
object HiddenTitleTail : VisualTransformation {
    override fun filter(text: AnnotatedString): TransformedText {
        val kept = keptOffsets(text.text)
        val drawn = kept.map(text.text::get).joinToString("")
        return TransformedText(AnnotatedString(drawn), KeptOffsetMapping(kept, text.length))
    }

    private fun keptOffsets(text: String): List<Int> {
        val kept = ArrayDeque<Int>(text.length)
        var inTail = true
        for (offset in text.indices.reversed()) {
            val char = text[offset]
            if (char == '\n') {
                inTail = true
            } else if (char != '.' && !char.isWhitespace()) {
                inTail = false
            }
            if (!inTail || char != '.') kept.addFirst(offset)
        }
        return kept
    }

    private class KeptOffsetMapping(
        private val kept: List<Int>,
        private val length: Int,
    ) : OffsetMapping {
        override fun originalToTransformed(offset: Int): Int = kept.count { it < offset }

        override fun transformedToOriginal(offset: Int): Int = kept.getOrElse(offset) { length }
    }
}
