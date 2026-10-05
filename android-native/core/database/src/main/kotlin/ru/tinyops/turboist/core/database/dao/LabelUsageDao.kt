package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.TaskStatus

/**
 * One tagging, with the facts about the task carrying it that the usage report
 * asks about.
 *
 * Deliberately not the task row: the report reads four of its columns and would
 * otherwise drag every task on the device into memory whole, titles, notes and
 * all, to count them.
 */
data class LabelTaggingFacts(
    val labelLocalId: Long,
    val taggedAt: Long?,
    val status: TaskStatus,
    val dueAt: Long?,
    val completedAt: Long?,
    val projectLocalId: Long?,
)

/**
 * What the label usage report is computed from.
 *
 * The report is worked out on the device rather than fetched, so it is there
 * with no connection like every other screen. That is only possible because the
 * moment a label was applied is replicated with the tagging itself: the windows
 * are counted from it, and without it the report could only ever be an opinion
 * about when the device happened to hear about the tagging.
 *
 * One standing query for the whole workspace, like the other joins a screen
 * needs beside its rows. A tagging is two ids and four small columns, there is
 * one per task-and-label pair, and a query that never changes shape is steadier
 * than one rebuilt around whichever period the user is looking at — switching
 * period must not cost a trip to the database, because the numbers for all three
 * windows are computed from the same rows.
 */
@Dao
interface LabelUsageDao {
    /**
     * Every tagging in the workspace, with its task's status, dates and project.
     *
     * A tagging whose task is gone cannot be here: the tagging table cascades
     * with the task, so there is nothing to count for work that no longer exists.
     */
    @Query(
        "SELECT tl.labelLocalId AS labelLocalId, tl.createdAt AS taggedAt, t.status AS status, " +
            "t.dueAt AS dueAt, t.completedAt AS completedAt, t.projectLocalId AS projectLocalId " +
            "FROM task_labels tl JOIN tasks t ON t.localId = tl.taskLocalId",
    )
    fun observeTaggings(): Flow<List<LabelTaggingFacts>>
}
