package ru.tinyops.turboist.nativeapp.ui.markdown

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp

/**
 * A description, rendered.
 *
 * Everything is one column of text blocks rather than a scrolling document: a
 * description belongs to the task it is on, and giving it a scroller of its own
 * would trap the gesture that scrolls the task.
 */
@Composable
fun MarkdownText(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.bodyMedium,
) {
    val blocks = Markdown.blocks(text)
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        for (block in blocks) {
            when (block) {
                is MarkdownBlock.Heading -> Text(text = annotate(block.segments), style = headingStyle(block.level))
                is MarkdownBlock.Paragraph -> Text(text = annotate(block.segments), style = style)
                is MarkdownBlock.Items -> ItemList(block, style)
            }
        }
    }
}

/**
 * A list, drawn with its own markers.
 *
 * The marker is a separate column so that an item running onto a second line
 * lines up under its own text rather than under the bullet.
 */
@Composable
private fun ItemList(
    block: MarkdownBlock.Items,
    style: TextStyle,
) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        block.items.forEachIndexed { index, item ->
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = if (block.ordered) "${index + 1}." else "•",
                    style = style,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(text = annotate(item), style = style)
            }
        }
    }
}

/** The six heading levels, mapped onto the type scale the rest of the screen uses. */
@Composable
private fun headingStyle(level: Int): TextStyle =
    when (level) {
        1 -> MaterialTheme.typography.titleLarge
        2 -> MaterialTheme.typography.titleMedium
        else -> MaterialTheme.typography.titleSmall
    }

/** Turns the runs of one block into styled text, links included. */
@Composable
private fun annotate(segments: List<InlineSegment>): AnnotatedString {
    val linkStyles =
        TextLinkStyles(
            style =
                SpanStyle(
                    color = MaterialTheme.colorScheme.primary,
                    textDecoration = TextDecoration.Underline,
                ),
        )
    val codeColor = MaterialTheme.colorScheme.onSurfaceVariant
    return buildAnnotatedString { appendSegments(segments, linkStyles, codeColor) }
}

private fun AnnotatedString.Builder.appendSegments(
    segments: List<InlineSegment>,
    linkStyles: TextLinkStyles,
    codeColor: Color,
) {
    for (segment in segments) {
        when (segment) {
            is InlineSegment.Text -> append(segment.value)
            is InlineSegment.Bold ->
                withStyle(SpanStyle(fontWeight = FontWeight.Bold)) {
                    appendSegments(segment.segments, linkStyles, codeColor)
                }

            is InlineSegment.Italic ->
                withStyle(SpanStyle(fontStyle = FontStyle.Italic)) {
                    appendSegments(segment.segments, linkStyles, codeColor)
                }

            is InlineSegment.Code ->
                withStyle(SpanStyle(fontFamily = FontFamily.Monospace, color = codeColor)) {
                    append(segment.value)
                }

            is InlineSegment.Link ->
                withLink(LinkAnnotation.Url(url = segment.href, styles = linkStyles)) {
                    append(segment.text)
                }
        }
    }
}
