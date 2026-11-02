package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.ui.labelTint
import ru.tinyops.turboist.nativeapp.tasks.PlanStatsHeader

/**
 * The line the planning screen leads with: how much of the planned week came
 * from the daily plan, then every label the planned work carries with its
 * count, most used first. Labels are written as hashtags in their own colour,
 * exactly as a row writes them, so a count reads as "this many of those".
 *
 * Draws nothing while there is nothing to say, so an empty plan does not open
 * with a blank band.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun PlanStatsBar(
    header: PlanStatsHeader,
    gutter: Dp,
) {
    val stats = header.stats
    val troiki = header.showTroiki && stats.total > 0
    if (stats.labels.isEmpty() && !troiki) return
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(horizontal = gutter + 4.dp, vertical = 6.dp),
    ) {
        if (troiki) {
            Text(
                text = stringResource(R.string.page_nextWeek_fromTroiki, stats.fromTroiki, stats.total),
                style = MaterialTheme.typography.labelMedium,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onSurface,
            )
        }
        for (usage in stats.labels) {
            Text(
                text = "#" + usage.label.name + " " + usage.count,
                style = MaterialTheme.typography.labelMedium,
                color = labelTint(usage.label) ?: MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
