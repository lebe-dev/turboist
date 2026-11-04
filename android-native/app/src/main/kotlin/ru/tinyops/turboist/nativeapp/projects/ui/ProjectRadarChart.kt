package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.core.model.view.ProjectRadar
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.RadarChart
import ru.tinyops.turboist.nativeapp.ui.RadarSpoke

/** Where the finished work went, project by project; see [RadarChart]. */
@Composable
fun ProjectRadarChart(
    radar: ProjectRadar,
    modifier: Modifier = Modifier,
) {
    val spokes =
        remember(radar) {
            radar.axes.map {
                RadarSpoke(
                    name = it.activity.project.title,
                    count = it.completed,
                    colorToken = it.activity.project.color,
                    angle = it.angle,
                    current = it.current,
                    previous = it.previous,
                )
            }
        }
    RadarChart(
        spokes = spokes,
        description = stringResource(R.string.page_projects_radar_aria),
        hiddenText =
            radar.hidden.takeIf { it > 0 }?.let { stringResource(R.string.page_projects_radar_hidden, it.toString()) },
        modifier = modifier,
    )
}
