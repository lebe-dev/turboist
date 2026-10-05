package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Embedded
import androidx.room.Query
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.model.RelationType

/** A label as it is carried by one task. */
data class TaskLabelJoin(
    val taskLocalId: Long,
    @Embedded val label: LabelRow,
)

/**
 * One relation edge, with the only thing about its endpoints a list needs: is
 * the task at the blocking end still open.
 */
data class TaskRelationEdge(
    val sourceTaskLocalId: Long,
    val targetTaskLocalId: Long,
    val type: RelationType,
    val sourceOpen: Boolean,
)

/** A subtask and the task above it. */
data class TaskParentLink(
    val taskLocalId: Long,
    val parentLocalId: Long,
)

/**
 * One still-open task standing in the way of another, named.
 *
 * A count is enough for a list row, which only draws a padlock; a screen that
 * refuses a completion has to say which work has to happen first, so the blocking
 * task's title travels with the edge.
 */
data class OpenBlocker(
    val blockedLocalId: Long,
    val blockerLocalId: Long,
    val blockerServerId: Long?,
    val blockerTitle: String,
)

/**
 * The three things a list needs that a task row does not carry.
 *
 * A task row holds its own columns and nothing else, so a list that wants to
 * draw chips, a padlock and a link count has to fetch them separately. Each one
 * is fetched whole rather than for the rows currently on screen: all three are
 * small next to the task table — a workspace has tens of labels, tens of
 * relations, and a parent link only for tasks that are subtasks — and one
 * standing query per screen is both cheaper and steadier than a query that is
 * rebuilt every time the list underneath it changes by a row.
 *
 * Nothing here computes what being blocked means. These are the facts; the rule
 * that turns them into a blocked task lives in the domain, next to the sort and
 * the grouping, where it is exercised without a database.
 */
@Dao
interface TaskHydrationDao {
    /** Every task-to-label tagging, with the label itself, ordered by label name. */
    @Query(
        "SELECT tl.taskLocalId AS taskLocalId, l.* FROM task_labels tl " +
            "JOIN labels l ON l.localId = tl.labelLocalId " +
            "ORDER BY l.name COLLATE NOCASE",
    )
    fun observeTaskLabels(): Flow<List<TaskLabelJoin>>

    /**
     * Every relation edge in the workspace.
     *
     * `sourceOpen` reads the status of the task at the source end, because that
     * is what decides whether a `blocks` edge still holds anything up: a blocker
     * that was completed — or cancelled, which also settles it — releases what
     * it was blocking.
     */
    @Query(
        "SELECT r.sourceTaskLocalId AS sourceTaskLocalId, r.targetTaskLocalId AS targetTaskLocalId, " +
            "r.type AS type, (s.status = 'open') AS sourceOpen " +
            "FROM task_relations r JOIN tasks s ON s.localId = r.sourceTaskLocalId",
    )
    fun observeRelationEdges(): Flow<List<TaskRelationEdge>>

    /**
     * The parent link of every subtask. Root tasks are absent rather than
     * listed with an empty parent, which keeps this to the handful of rows that
     * have something to say.
     */
    @Query("SELECT localId AS taskLocalId, parentLocalId AS parentLocalId FROM tasks WHERE parentLocalId IS NOT NULL")
    fun observeParentLinks(): Flow<List<TaskParentLink>>

    /**
     * Every `blocks` edge whose blocker is still open, with that blocker named.
     *
     * Only the open ones: a blocker that was completed — or cancelled, which
     * settles it just as finally — no longer holds anything up, and listing it
     * would tell the user to finish something that is already done. Fetched for
     * the whole workspace like the other standing queries here, because the edges
     * are few and one query that never changes shape is steadier than one rebuilt
     * around whichever task is on screen.
     */
    @Query(
        "SELECT r.targetTaskLocalId AS blockedLocalId, s.localId AS blockerLocalId, " +
            "s.serverId AS blockerServerId, s.title AS blockerTitle " +
            "FROM task_relations r JOIN tasks s ON s.localId = r.sourceTaskLocalId " +
            "WHERE r.type = 'blocks' AND s.status = 'open' " +
            "ORDER BY r.createdAt, r.localId",
    )
    fun observeOpenBlockers(): Flow<List<OpenBlocker>>
}
