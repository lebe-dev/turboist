package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.TaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskRow

@Dao
abstract class TaskDao {
    @Insert
    abstract suspend fun insert(row: TaskRow): Long

    @Update
    abstract suspend fun update(row: TaskRow)

    @Delete
    abstract suspend fun delete(row: TaskRow)

    @Query("SELECT localId FROM tasks WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * The device id of the task the server calls this, and again if that changes.
     *
     * A link from outside names a task the server's way, and the row it points at
     * may not have arrived yet — or may arrive a moment later, or be replaced by a
     * fresh sync. Watching the lookup instead of asking it once is what lets a
     * link opened just ahead of the data resolve itself rather than staying dead.
     */
    @Query("SELECT localId FROM tasks WHERE serverId = :serverId")
    abstract fun observeLocalIdForServerId(serverId: Long): Flow<Long?>

    /** The device ids of every named record the replica already holds. */
    @Query("SELECT localId, serverId FROM tasks WHERE serverId IN (:serverIds)")
    abstract suspend fun identitiesForServerIds(serverIds: Collection<Long>): List<TaskIdentity>

    @Query("SELECT * FROM tasks WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): TaskRow?

    @Query("SELECT * FROM tasks WHERE serverId = :serverId")
    abstract suspend fun byServerId(serverId: Long): TaskRow?

    @Query("SELECT * FROM tasks WHERE localId = :localId")
    abstract fun observeByLocalId(localId: Long): Flow<TaskRow?>

    @Query("SELECT * FROM tasks WHERE parentLocalId = :parentLocalId ORDER BY localId")
    abstract suspend fun subtasksOf(parentLocalId: Long): List<TaskRow>

    /**
     * The still-open tasks filed in a project, subtasks included.
     *
     * A subtask carries its parent's project, so one query names the whole of a
     * project's live work. Finished work is left out: it is history, and a rule
     * that rewrote it would be rewriting the record of what was done.
     */
    @Query("SELECT * FROM tasks WHERE projectLocalId = :projectLocalId AND status = 'open'")
    abstract suspend fun openInProject(projectLocalId: Long): List<TaskRow>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM tasks WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM tasks WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    /**
     * The tasks that were finished before [cutoff].
     *
     * The device keeps a bounded stretch of finished work, and this is what has
     * fallen out of the back of it. Only completions: a cancelled task carries no
     * completion date, and an open one is live work whatever its age.
     */
    @Query("SELECT localId FROM tasks WHERE completedAt < :cutoff AND status = 'completed'")
    abstract suspend fun completedBefore(cutoff: Long): List<Long>

    /**
     * The tasks sitting directly under any of [parentLocalIds], each named with
     * the task above it.
     *
     * Asked before removing rows, because removing a task removes everything
     * beneath it: the caller has to know what would go with it before deciding
     * that it may go at all.
     */
    @Query(
        "SELECT localId AS taskLocalId, parentLocalId AS parentLocalId FROM tasks " +
            "WHERE parentLocalId IN (:parentLocalIds)",
    )
    abstract suspend fun childrenOf(parentLocalIds: Collection<Long>): List<TaskParentLink>

    /** Removes the named tasks, and with each of them everything beneath it. */
    @Query("DELETE FROM tasks WHERE localId IN (:localIds)")
    abstract suspend fun deleteByLocalIds(localIds: Collection<Long>): Int

    @Query("SELECT COUNT(*) FROM tasks")
    abstract suspend fun count(): Int

    /**
     * How many tasks are pinned. The pinned shelf has a cap, and this is what a
     * pin is checked against before it is allowed — a completed task keeps its
     * pin and keeps taking up a place, exactly as it does on the server.
     */
    @Query("SELECT COUNT(*) FROM tasks WHERE isPinned = 1")
    abstract suspend fun countPinned(): Int

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created task becomes addressable — until it
     * runs, the row exists only here and has no link to share. The local id is
     * untouched, which is the whole point: the screen that created the task is
     * still pointing at it.
     */
    @Query("UPDATE tasks SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    // --- the recorded runs of a repeating task ---

    /**
     * Whether a run of this repeating task is already recorded as finished
     * somewhere in [from] until [until].
     *
     * A repeating task stays open when it is ticked off — it moves on to its next
     * occurrence — so the run just finished is kept as a separate row. Ticking the
     * same task off twice in one day must not record two of them, and must not
     * move the task on twice either, so this is what the second tick is answered
     * with.
     */
    @Query(
        "SELECT EXISTS(SELECT 1 FROM tasks WHERE sourceTaskLocalId = :sourceTaskLocalId " +
            "AND completedAt >= :from AND completedAt < :until)",
    )
    abstract suspend fun hasRecurrenceCompletionBetween(
        sourceTaskLocalId: Long,
        from: Long,
        until: Long,
    ): Boolean

    /**
     * The recorded run this device wrote for itself and the server has not yet
     * named, if there is one.
     *
     * A run finished offline is recorded here and again on the server when the
     * completion is sent, and the two are the same event: the server is told the
     * exact moment, so the moment is what identifies them as one. Finding the
     * local row lets the server's version replace it instead of sitting beside it
     * as a second entry in the history for ever.
     */
    @Query(
        "SELECT * FROM tasks WHERE sourceTaskLocalId = :sourceTaskLocalId " +
            "AND completedAt = :completedAt AND serverId IS NULL LIMIT 1",
    )
    abstract suspend fun unnamedRecurrenceCompletion(
        sourceTaskLocalId: Long,
        completedAt: Long,
    ): TaskRow?

    @Transaction
    open suspend fun upsertByServerId(row: TaskRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)

    // --- labels attached to a task ---

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    abstract suspend fun insertLabels(rows: List<TaskLabelRow>)

    @Query("DELETE FROM task_labels WHERE taskLocalId = :taskLocalId AND labelLocalId NOT IN (:keep)")
    abstract suspend fun deleteLabelsExcept(
        taskLocalId: Long,
        keep: Collection<Long>,
    )

    @Query("DELETE FROM task_labels WHERE taskLocalId = :taskLocalId")
    abstract suspend fun clearLabels(taskLocalId: Long)

    @Query("SELECT * FROM task_labels WHERE taskLocalId = :taskLocalId")
    abstract suspend fun labelsOf(taskLocalId: Long): List<TaskLabelRow>

    /**
     * Replaces a task's labels with exactly [labelLocalIds], as a diff.
     *
     * A label that is on the task before and after keeps the moment it was
     * attached. Rewriting every edge with the current time would be invisible on
     * screen and would quietly destroy the only data the label usage report is
     * computed from — the same reason the server diffs rather than replaces.
     */
    @Transaction
    open suspend fun setLabels(
        taskLocalId: Long,
        labelLocalIds: Collection<Long>,
        taggedAt: Long,
    ) {
        val wanted = labelLocalIds.distinct()
        if (wanted.isEmpty()) {
            clearLabels(taskLocalId)
            return
        }
        deleteLabelsExcept(taskLocalId, wanted)
        insertLabels(wanted.map { TaskLabelRow(taskLocalId, it, taggedAt) })
    }
}

/** A task's two identities, for resolving a page of incoming records in one query. */
data class TaskIdentity(
    val localId: Long,
    val serverId: Long?,
)
