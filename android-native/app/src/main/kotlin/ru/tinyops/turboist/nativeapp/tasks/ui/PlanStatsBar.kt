package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.ui.labelTint
import ru.tinyops.turboist.nativeapp.tasks.PlanStatsHeader

/** One share of the bar: a label, or the planned work that carries none. */
private data class Share(
    val key: String,
    val name: String,
    val count: Int,
    val color: Color,
    val unlabeled: Boolean = false,
)

/**
 * The week view's answer to "what is the planned week made of": one bar cut
 * into a share per label, in the label's own colour and most used first, the
 * work with no label as a grey share at the end, and a legend under it. Each
 * label use is one share and each unlabeled task is one more, so the gaps are
 * on the bar as plainly as the labels are.
 *
 * Beside the legend, when the workspace keeps a daily plan, a small meter says
 * how much of the planned work came from projects standing in it.
 *
 * Draws nothing while nothing is planned, so an empty week does not open with
 * an empty bar.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun PlanStatsBar(
    header: PlanStatsHeader,
    gutter: Dp,
) {
    val stats = header.stats
    if (stats.total == 0) return
    val shares = shares(header)
    val units = shares.sumOf { it.count }.coerceAtLeast(1)
    val track = MaterialTheme.colorScheme.surfaceVariant
    val muted = MaterialTheme.colorScheme.onSurfaceVariant

    Column(
        verticalArrangement = Arrangement.spacedBy(6.dp),
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(horizontal = gutter + 4.dp, vertical = 6.dp),
    ) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(2.dp),
            modifier =
                Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(CircleShape)
                    .background(track),
        ) {
            for (share in shares) {
                Box(
                    Modifier
                        .weight(share.count.toFloat() / units)
                        .fillMaxHeight()
                        .background(share.color),
                )
            }
        }
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
            itemVerticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.fillMaxWidth(),
        ) {
            for (share in shares.take(LEGEND_LIMIT)) LegendEntry(share)
            val hidden = shares.size - LEGEND_LIMIT
            if (hidden > 0) {
                Text(
                    text = stringResource(R.string.page_week_planStats_more, hidden),
                    style = MaterialTheme.typography.labelMedium,
                    color = muted,
                )
            }
            if (header.showTroiki) TroikiMeter(stats.fromTroiki, stats.total)
        }
    }
}

@Composable
private fun shares(header: PlanStatsHeader): List<Share> {
    val fallback = MaterialTheme.colorScheme.primary
    val labelled =
        header.stats.labels.map { usage ->
            Share(
                key = "label-" + usage.label.localId,
                name = usage.label.name,
                count = usage.count,
                color = labelTint(usage.label) ?: fallback,
            )
        }
    if (header.stats.unlabeled == 0) return labelled
    return labelled +
        Share(
            key = "unlabeled",
            name = stringResource(R.string.page_week_planStats_unlabeled),
            count = header.stats.unlabeled,
            color = MaterialTheme.colorScheme.outline,
            unlabeled = true,
        )
}

@Composable
private fun LegendEntry(share: Share) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        Box(
            Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(share.color),
        )
        Text(
            text = share.name,
            style = MaterialTheme.typography.labelMedium,
            fontStyle = if (share.unlabeled) FontStyle.Italic else FontStyle.Normal,
            color =
                if (share.unlabeled) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
        )
        Text(
            text = share.count.toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun TroikiMeter(
    fromTroiki: Int,
    total: Int,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(
            text = stringResource(R.string.page_week_planStats_fromTroiki),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Box(
            Modifier
                .width(48.dp)
                .height(6.dp)
                .clip(CircleShape)
                .background(MaterialTheme.colorScheme.surfaceVariant),
        ) {
            Box(
                Modifier
                    .fillMaxWidth(fromTroiki.toFloat() / total)
                    .fillMaxHeight()
                    .background(MaterialTheme.colorScheme.primary),
            )
        }
        Text(
            text = "$fromTroiki/$total",
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurface,
        )
    }
}

private const val LEGEND_LIMIT = 6
