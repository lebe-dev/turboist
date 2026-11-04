package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Project
import java.time.Instant
import java.time.ZoneId
import kotlin.math.PI

/**
 * One task finished inside a project, with the moment it was finished.
 *
 * Only finished work is reported: the activity radar is about where the work
 * went, and abandoning a task is not doing it. The caller leaves cancelled tasks
 * out, so a row here is always a completion.
 */
data class ProjectCompletion(
    val projectLocalId: Long,
    val completedAt: Long,
)

/**
 * One window's counters for a single project: tasks finished inside it, and
 * inside the equally long window immediately before it.
 */
data class ProjectPeriodActivity(
    val completed: Int = 0,
    val previousCompleted: Int = 0,
)

/**
 * What the radar knows about one project.
 *
 * The windows are the label report's — rolling 7 / 30 / 90 days ending tonight
 * ([LabelUsagePeriod]) — and are cut in the same zone, so the two radars agree
 * about what "this week" is.
 */
data class ProjectActivity(
    val project: Project,
    val periods: Map<LabelUsagePeriod, ProjectPeriodActivity> = emptyMap(),
) {
    /** This project's counters for one window; zeroes for a project that did nothing in it. */
    fun period(period: LabelUsagePeriod): ProjectPeriodActivity = periods[period] ?: ProjectPeriodActivity()
}

/**
 * The activity of every project, counted over the replica.
 *
 * Every project is returned, the quiet ones with zeroes: which of them earn a
 * spoke is the radar's decision, not the count's.
 */
fun projectActivity(
    projects: List<Project>,
    completions: Collection<ProjectCompletion>,
    now: Instant,
    zone: ZoneId,
): List<ProjectActivity> {
    val windows = LabelUsagePeriod.entries.associateWith { labelUsageWindow(now, zone, it) }
    val byProject = completions.groupBy { it.projectLocalId }
    return projects.map { project ->
        val own = byProject[project.localId].orEmpty()
        ProjectActivity(
            project = project,
            periods =
                windows.mapValues { (_, window) ->
                    val previous = previousWindow(window)
                    ProjectPeriodActivity(
                        completed = own.count { it.completedAt in window },
                        previousCompleted = own.count { it.completedAt in previous },
                    )
                },
        )
    }
}

/**
 * One spoke of the project radar. [current] and [previous] are fractions of the
 * chart's radius (0..1) for the chosen window and the one before it; [angle] is
 * in radians, clockwise from twelve o'clock.
 */
data class ProjectRadarAxis(
    val activity: ProjectActivity,
    val completed: Int,
    val angle: Double,
    val current: Float,
    val previous: Float,
)

/**
 * The projects laid out as directions the finished work went in.
 *
 * [scaleMax] is the completion count the outer ring stands for; [hidden] is how
 * many projects with activity were left without a spoke because of the cap.
 */
data class ProjectRadar(
    val axes: List<ProjectRadarAxis>,
    val scaleMax: Int,
    val hidden: Int,
)

/**
 * Lays the projects out as a radar, exactly as [labelRadar] does for labels and
 * as the web client does for projects: a project earns a spoke when it finished
 * something in either window, the busiest win past [maxAxes], and spokes are
 * placed by title so a project keeps its direction when the period changes. Null
 * when there are too few spokes to make a shape.
 */
fun projectRadar(
    rows: List<ProjectActivity>,
    period: LabelUsagePeriod,
    maxAxes: Int = LABEL_RADAR_MAX_AXES,
): ProjectRadar? {
    fun weight(row: ProjectActivity) = maxOf(row.period(period).completed, row.period(period).previousCompleted)
    val candidates =
        rows
            .filter { weight(it) > 0 }
            .sortedWith(compareByDescending<ProjectActivity> { weight(it) }.thenBy { it.project.title.lowercase() })
    val picked =
        candidates
            .take(maxAxes)
            .sortedWith(compareBy<ProjectActivity> { it.project.title.lowercase() }.thenBy { it.project.localId })
    if (picked.size < LABEL_RADAR_MIN_AXES) return null
    val scaleMax = maxOf(1, picked.maxOf { weight(it) })
    return ProjectRadar(
        axes =
            picked.mapIndexed { index, row ->
                ProjectRadarAxis(
                    activity = row,
                    completed = row.period(period).completed,
                    angle = 2 * PI * index / picked.size,
                    current = row.period(period).completed.toFloat() / scaleMax,
                    previous = row.period(period).previousCompleted.toFloat() / scaleMax,
                )
            },
        scaleMax = scaleMax,
        hidden = candidates.size - picked.size,
    )
}
