package ru.tinyops.turboist.nativeapp.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextMeasurer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ui.projectTint
import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.sin

/**
 * Which way the work pulls: one spoke per label or project, the chosen window as
 * a filled shape and the window before it as a dashed outline.
 *
 * The names are painted rather than laid out as text nodes — they repeat the
 * list underneath, which is where an item is opened from — so the chart is one
 * image to accessibility services, described as a whole by [description].
 * [hiddenText] is the legend's note about items left off the chart, or null when
 * none were.
 */
@Composable
fun RadarChart(
    spokes: List<RadarSpoke>,
    description: String,
    hiddenText: String?,
    modifier: Modifier = Modifier,
) {
    val measurer = rememberTextMeasurer()
    val colors = MaterialTheme.colorScheme
    val nameStyle = MaterialTheme.typography.labelSmall.copy(color = colors.onSurfaceVariant)
    val countStyle = MaterialTheme.typography.labelSmall.copy(color = colors.onSurface)
    val palette =
        RadarPalette(
            grid = colors.outlineVariant,
            current = colors.primary,
            previous = colors.onSurfaceVariant,
            dotRim = colors.surface,
        )
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Canvas(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .aspectRatio(CHART_ASPECT)
                    .semantics { contentDescription = description },
        ) {
            val radius = min(size.width / 2f - LABEL_ROOM.toPx(), size.height / 2f - LABEL_GAP.toPx() * 2)
            val chart = RadarGeometry(center, radius.coerceAtLeast(1f), spokes)
            drawGrid(chart, palette)
            drawShapes(chart, palette)
            drawNames(chart, measurer, nameStyle, countStyle, LABEL_GAP.toPx())
        }
        Legend(hiddenText)
    }
}

private data class RadarPalette(
    val grid: Color,
    val current: Color,
    val previous: Color,
    val dotRim: Color,
)

private class RadarGeometry(
    val center: Offset,
    val radius: Float,
    val spokes: List<RadarSpoke>,
) {
    fun point(
        angle: Double,
        fraction: Float,
        reach: Float = radius,
    ): Offset =
        Offset(
            center.x + (sin(angle) * fraction * reach).toFloat(),
            center.y - (cos(angle) * fraction * reach).toFloat(),
        )

    fun shape(fractionOf: (Int) -> Float): Path =
        Path().apply {
            spokes.forEachIndexed { index, axis ->
                val at = point(axis.angle, fractionOf(index))
                if (index == 0) moveTo(at.x, at.y) else lineTo(at.x, at.y)
            }
            close()
        }
}

private fun DrawScope.drawGrid(
    chart: RadarGeometry,
    palette: RadarPalette,
) {
    val hairline = 1.dp.toPx()
    for (ring in RINGS) {
        drawPath(chart.shape { ring }, palette.grid, style = Stroke(width = hairline))
    }
    for (axis in chart.spokes) {
        drawLine(palette.grid, chart.center, chart.point(axis.angle, 1f), strokeWidth = hairline)
    }
}

private fun DrawScope.drawShapes(
    chart: RadarGeometry,
    palette: RadarPalette,
) {
    val axes = chart.spokes
    drawPath(
        chart.shape { axes[it].previous },
        palette.previous,
        style =
            Stroke(
                width = 1.5.dp.toPx(),
                join = StrokeJoin.Round,
                pathEffect = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 3.dp.toPx())),
            ),
    )
    val current = chart.shape { axes[it].current }
    drawPath(current, palette.current.copy(alpha = FILL_ALPHA))
    drawPath(current, palette.current, style = Stroke(width = 2.dp.toPx(), join = StrokeJoin.Round))
    for (axis in axes) {
        val at = chart.point(axis.angle, axis.current)
        drawCircle(palette.dotRim, radius = DOT_RADIUS.toPx() + 1.5.dp.toPx(), center = at)
        drawCircle(
            projectTint(axis.colorToken) ?: palette.current,
            radius = DOT_RADIUS.toPx(),
            center = at,
        )
    }
}

/**
 * Each name sits just past the rim with its count after it, and grows away from
 * the chart: to the right on the right half, to the left on the left, centred at
 * the top and bottom.
 */
private fun DrawScope.drawNames(
    chart: RadarGeometry,
    measurer: TextMeasurer,
    nameStyle: TextStyle,
    countStyle: TextStyle,
    gap: Float,
) {
    val maxWidth = maxOf(size.width / 2f - chart.radius - gap, size.width * NAME_MIN_SHARE).toInt()
    for (axis in chart.spokes) {
        val text =
            buildAnnotatedString {
                append(axis.name)
                withStyle(countStyle.toSpanStyle()) { append(" " + axis.count) }
            }
        val measured =
            measurer.measure(
                text = text,
                style = nameStyle,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                constraints = Constraints(maxWidth = maxWidth),
            )
        val tip = chart.point(axis.angle, 1f, chart.radius + gap)
        val dx = sin(axis.angle).toFloat()
        val dy = -cos(axis.angle).toFloat()
        val x =
            when {
                abs(dx) < SIDE_THRESHOLD -> tip.x - measured.size.width / 2f
                dx > 0 -> tip.x
                else -> tip.x - measured.size.width
            }
        val y =
            when {
                abs(dy) < MIDDLE_THRESHOLD -> tip.y - measured.size.height / 2f
                dy > 0 -> tip.y
                else -> tip.y - measured.size.height
            }
        drawText(measured, topLeft = Offset(x, y))
    }
}

@Composable
private fun Legend(hiddenText: String?) {
    val colors = MaterialTheme.colorScheme
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterHorizontally),
        verticalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        LegendEntry(stringResource(R.string.page_labels_radar_current)) {
            drawRect(colors.primary.copy(alpha = FILL_ALPHA))
            drawRect(colors.primary, style = Stroke(width = 2.dp.toPx()))
        }
        LegendEntry(stringResource(R.string.page_labels_radar_previous)) {
            drawLine(
                colors.onSurfaceVariant,
                Offset(0f, size.height / 2f),
                Offset(size.width, size.height / 2f),
                strokeWidth = 2.dp.toPx(),
                pathEffect = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 3.dp.toPx())),
            )
        }
        if (hiddenText != null) {
            Text(
                text = hiddenText,
                style = MaterialTheme.typography.labelSmall,
                color = colors.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun LegendEntry(
    text: String,
    swatch: DrawScope.() -> Unit,
) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Canvas(modifier = Modifier.size(width = 16.dp, height = 10.dp), onDraw = swatch)
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** Four evenly spaced rings: a quarter of the busiest spoke's count each. */
private const val RING_COUNT = 4
private val RINGS = (1..RING_COUNT).map { it.toFloat() / RING_COUNT }

/** A name is never squeezed narrower than a third of the chart, even when the rim is close. */
private const val NAME_MIN_SHARE = 1f / 3f
private const val CHART_ASPECT = 1.35f
private val LABEL_ROOM = 72.dp
private val LABEL_GAP = 10.dp
private val DOT_RADIUS = 3.5.dp
private const val FILL_ALPHA = 0.18f
private const val SIDE_THRESHOLD = 0.2f
private const val MIDDLE_THRESHOLD = 0.3f
