package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.core.model.view.LabelRadar
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.RadarChart
import ru.tinyops.turboist.nativeapp.ui.RadarSpoke

/** Which way the work pulls, label by label; see [RadarChart]. */
@Composable
fun LabelRadarChart(
    radar: LabelRadar,
    modifier: Modifier = Modifier,
) {
    val spokes =
        remember(radar) {
            radar.axes.map {
                RadarSpoke(
                    name = it.usage.label.name,
                    count = it.applied,
                    colorToken = it.usage.label.color,
                    angle = it.angle,
                    current = it.current,
                    previous = it.previous,
                )
            }
        }
    RadarChart(
        spokes = spokes,
        description = stringResource(R.string.page_labels_radar_aria),
        // The shared wording carries no type for its values, so the count is
        // handed over as text.
        hiddenText =
            radar.hidden.takeIf { it > 0 }?.let { stringResource(R.string.page_labels_radar_hidden, it.toString()) },
        modifier = modifier,
    )
}
