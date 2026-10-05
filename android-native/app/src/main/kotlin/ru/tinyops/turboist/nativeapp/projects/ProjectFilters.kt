package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TroikiCategory

/**
 * The narrowings the projects screen offers, in the order they are drawn.
 *
 * Two of them ask about the kind of work and three about where the project
 * stands, which is why they are one list rather than two: at any moment the user
 * is looking at exactly one slice of the workspace, and offering two independent
 * pickers would let them build slices that answer nothing.
 */
enum class ProjectFilter {
    ALL,
    GENERIC,
    SOFTWARE,
    ARCHIVED,
    CANCELLED,
    COMPLETED,
    ;

    fun matches(project: Project): Boolean =
        when (this) {
            ALL -> true
            GENERIC -> project.type == ProjectType.GENERIC
            SOFTWARE -> project.type == ProjectType.SOFTWARE
            ARCHIVED -> project.status == ProjectStatus.ARCHIVED
            CANCELLED -> project.status == ProjectStatus.CANCELLED
            COMPLETED -> project.status == ProjectStatus.COMPLETED
        }

    /**
     * True while the filter is about the kind of work rather than about where a
     * project stands. Those are the slices that still hold a mix of open and
     * finished projects, and so the ones where the daily plan is worth sorting
     * by — inside a slice of finished projects it would order nothing.
     */
    val spansStatuses: Boolean get() = this == ALL || this == GENERIC || this == SOFTWARE
}

/**
 * The order projects are listed in, which is the web client's own.
 *
 * Open work leads, whatever else is true of it: a finished project is history,
 * and history belongs under the work. Inside the open ones the daily plan comes
 * first, in the order its three slots are worked through, because a project
 * committed to today is the one the user is looking for. Everything else is
 * alphabetical, which is the only order a person can predict.
 */
fun projectsInReadingOrder(
    projects: List<Project>,
    filter: ProjectFilter,
): List<Project> =
    projects.sortedWith(
        compareBy<Project> { it.status != ProjectStatus.OPEN }
            .thenBy { if (filter.spansStatuses) troikiRank(it.troikiCategory) else 0 }
            .thenBy(String.CASE_INSENSITIVE_ORDER) { it.title },
    )

/** Where a slot of the daily plan sits in the reading order; unplanned work sorts last. */
private fun troikiRank(category: TroikiCategory?): Int =
    when (category) {
        TroikiCategory.IMPORTANT -> 0
        TroikiCategory.MEDIUM -> 1
        TroikiCategory.REST -> 2
        // A slot spelling this build does not know, and no slot at all, both
        // sort after the three it does: an unrankable project belongs at the
        // bottom of the plan rather than at the top of it.
        TroikiCategory.UNKNOWN, null -> 3
    }
