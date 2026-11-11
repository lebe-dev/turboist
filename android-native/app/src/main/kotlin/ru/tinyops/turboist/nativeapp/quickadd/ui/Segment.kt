package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.layout.Measurable
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/** One choice of a [FittedSegments] control. */
internal data class Segment(
    val label: String,
    val selected: Boolean,
    val container: Color,
    val content: Color,
    val leading: (@Composable () -> Unit)? = null,
    val onClick: () -> Unit,
)

/**
 * A row of choices sized to their wording rather than cut into equal parts.
 *
 * Equal parts look tidy only while every label is about as long as the others;
 * "No date" beside "Date", in Russian or at a large font size, is not, and an
 * equal split cuts the long ones short while the short ones float in space. Here
 * every choice first gets the width its label needs, and whatever is left is
 * shared out evenly. When even that does not fit — a narrow phone, a large font —
 * the choices fold into two rows instead of being truncated.
 */
@Composable
internal fun FittedSegments(
    segments: List<Segment>,
    modifier: Modifier = Modifier,
    gap: Dp = 4.dp,
) {
    Layout(
        modifier = modifier.selectableGroup(),
        content = {
            for (segment in segments) SegmentButton(segment)
        },
    ) { measurables, constraints ->
        val gapPx = gap.roundToPx()
        val total = constraints.maxWidth
        val natural = measurables.map { it.maxIntrinsicWidth(Constraints.Infinity) }
        val all = measurables.indices.toList()
        val rows =
            if (natural.sum() + gapPx * (measurables.size - 1) <= total || measurables.size < MIN_TO_FOLD) {
                listOf(all)
            } else {
                all.chunked((measurables.size + 1) / 2)
            }
        val placed =
            rows.map { row ->
                val space = total - gapPx * (row.size - 1)
                val widths = shareOut(row.map { natural[it] }, space)
                val height = row.indices.maxOf { measurables[row[it]].minIntrinsicHeight(widths[it]) }
                row.indices.map { i ->
                    measurables[row[i]].measureFixed(widths[i], height)
                }
            }
        val height = placed.sumOf { row -> row.maxOf { it.height } } + gapPx * (placed.size - 1)
        layout(total, height) {
            var y = 0
            for (row in placed) {
                var x = 0
                for (placeable in row) {
                    placeable.place(x, y)
                    x += placeable.width + gapPx
                }
                y += row.maxOf { it.height } + gapPx
            }
        }
    }
}

private fun Measurable.measureFixed(
    width: Int,
    height: Int,
) = measure(Constraints.fixed(width, height))

/**
 * Widths for a row: each its natural width plus an even share of what is left,
 * or, when there is not enough, the space cut in proportion to what each needs.
 */
private fun shareOut(
    natural: List<Int>,
    space: Int,
): List<Int> {
    val needed = natural.sum()
    if (needed <= 0) return natural.map { space / natural.size }
    val extra = space - needed
    if (extra >= 0) {
        val share = extra / natural.size
        val widths = natural.map { it + share }.toMutableList()
        widths[widths.lastIndex] += space - widths.sum()
        return widths
    }
    val widths = natural.map { (it.toLong() * space / needed).toInt() }.toMutableList()
    widths[widths.lastIndex] += space - widths.sum()
    return widths
}

@Composable
private fun SegmentButton(segment: Segment) {
    Surface(
        selected = segment.selected,
        onClick = segment.onClick,
        shape = RoundedCornerShape(12.dp),
        color = segment.container,
        contentColor = segment.content,
        modifier = Modifier.heightIn(min = 44.dp),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp, Alignment.CenterHorizontally),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            segment.leading?.invoke()
            Text(
                text = segment.label,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/** Fewer choices than this always stay on one line. */
private const val MIN_TO_FOLD = 3
