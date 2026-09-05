package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.TaskTemplateLabelRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskRow

@Dao
abstract class TaskTemplateDao {
    @Insert
    abstract suspend fun insert(row: TaskTemplateRow): Long

    @Update
    abstract suspend fun update(row: TaskTemplateRow)

    @Delete
    abstract suspend fun delete(row: TaskTemplateRow)

    @Query("SELECT localId FROM task_templates WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE task_templates SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    @Query("SELECT * FROM task_templates WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): TaskTemplateRow?

    @Query("SELECT * FROM task_templates ORDER BY position, localId")
    abstract fun observeAll(): Flow<List<TaskTemplateRow>>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM task_templates WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM task_templates WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    @Query("SELECT COUNT(*) FROM task_templates")
    abstract suspend fun count(): Int

    @Transaction
    open suspend fun upsertByServerId(row: TaskTemplateRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)

    // --- subtasks ---

    @Insert
    abstract suspend fun insertSubtask(row: TaskTemplateSubtaskRow): Long

    @Query("SELECT localId FROM task_template_subtasks WHERE serverId = :serverId")
    abstract suspend fun subtaskLocalIdForServerId(serverId: Long): Long?

    @Query("SELECT * FROM task_template_subtasks WHERE templateLocalId = :templateLocalId ORDER BY position, localId")
    abstract suspend fun subtasksOf(templateLocalId: Long): List<TaskTemplateSubtaskRow>

    /**
     * Every checklist line the replica holds, template by template.
     *
     * Read in one go rather than per template: the screen that shows templates
     * shows all of them with their lines, and a query per template would be a
     * standing query per row that has to be torn down and set up again whenever
     * a template is added.
     */
    @Query("SELECT * FROM task_template_subtasks ORDER BY templateLocalId, position, localId")
    abstract fun observeAllSubtasks(): Flow<List<TaskTemplateSubtaskRow>>

    @Query("DELETE FROM task_template_subtasks WHERE templateLocalId = :templateLocalId")
    abstract suspend fun clearSubtasks(templateLocalId: Long)

    // --- labels, on the template and on its subtasks ---

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    abstract suspend fun insertLabels(rows: List<TaskTemplateLabelRow>)

    @Query("DELETE FROM task_template_labels WHERE templateLocalId = :templateLocalId")
    abstract suspend fun clearLabels(templateLocalId: Long)

    @Query("SELECT labelLocalId FROM task_template_labels WHERE templateLocalId = :templateLocalId")
    abstract suspend fun labelLocalIds(templateLocalId: Long): List<Long>

    /** Every label edge on every template, for the same reason the lines are read in one go. */
    @Query("SELECT * FROM task_template_labels")
    abstract fun observeAllLabels(): Flow<List<TaskTemplateLabelRow>>

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    abstract suspend fun insertSubtaskLabels(rows: List<TaskTemplateSubtaskLabelRow>)

    @Query("SELECT labelLocalId FROM task_template_subtask_labels WHERE subtaskLocalId = :subtaskLocalId")
    abstract suspend fun subtaskLabelLocalIds(subtaskLocalId: Long): List<Long>

    /** Every label edge on every checklist line, likewise. */
    @Query("SELECT * FROM task_template_subtask_labels")
    abstract fun observeAllSubtaskLabels(): Flow<List<TaskTemplateSubtaskLabelRow>>

    /** Replaces a template's own labels with exactly [labelLocalIds]. */
    @Transaction
    open suspend fun setLabels(
        templateLocalId: Long,
        labelLocalIds: Collection<Long>,
    ) {
        clearLabels(templateLocalId)
        insertLabels(labelLocalIds.distinct().map { TaskTemplateLabelRow(templateLocalId, it) })
    }
}
