package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus

/** One label and how many of the planned tasks carry it. */
data class PlannedLabelCount(
    val label: Label,
    val count: Int,
)

/**
 * What the week being planned is made of.
 *
 * @property labels the labels the open planned tasks carry, most used first and
 *   ties by name, so the order does not shuffle between two equal counts.
 * @property unlabeled how many of those tasks carry no label at all.
 * @property fromTroiki how many of those tasks belong to a project standing in
 *   a daily-plan bucket.
 * @property total how many open planned tasks the counters were taken over.
 */
data class WeekPlanStats(
    val labels: List<PlannedLabelCount>,
    val fromTroiki: Int,
    val total: Int,
    val unlabeled: Int = 0,
) {
    companion object {
        val EMPTY = WeekPlanStats(emptyList(), 0, 0)
    }
}

/**
 * Summarises the planned week the same way the web week view does, so the
 * screen can show which labels dominate it. The caller passes only tasks planned
 * for the week.
 *
 * Subtasks count on their own: planning a parent cascades onto them, so they
 * are planned work too. Finished work is left out — the plan is about what is
 * still ahead.
 */
fun weekPlanStats(
    tasks: List<Task>,
    troikiProjectLocalIds: Set<Long>,
): WeekPlanStats {
    val open = tasks.filter { it.status == TaskStatus.OPEN }
    val labels =
        open
            .flatMap { it.labels }
            .groupBy { it.localId }
            .map { (_, carried) -> PlannedLabelCount(carried.first(), carried.size) }
            .sortedWith(compareByDescending<PlannedLabelCount> { it.count }.thenBy { it.label.name })
    val fromTroiki = open.count { task -> task.projectLocalId?.let { it in troikiProjectLocalIds } == true }
    return WeekPlanStats(
        labels = labels,
        fromTroiki = fromTroiki,
        total = open.size,
        unlabeled = open.count { it.labels.isEmpty() },
    )
}
