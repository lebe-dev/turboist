package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.ProjectLabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.model.TroikiCategory

@Dao
abstract class ProjectDao {
    @Insert
    abstract suspend fun insert(row: ProjectRow): Long

    @Update
    abstract suspend fun update(row: ProjectRow)

    @Delete
    abstract suspend fun delete(row: ProjectRow)

    @Query("SELECT localId FROM projects WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE projects SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    @Query("SELECT * FROM projects WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): ProjectRow?

    @Query("SELECT * FROM projects WHERE serverId = :serverId")
    abstract suspend fun byServerId(serverId: Long): ProjectRow?

    /**
     * One project, and again whenever it changes.
     *
     * A screen showing a single project is a standing query like every list, so
     * renaming it, finishing it or pinning it — here or on another device —
     * redraws the screen without anything asking it to.
     */
    @Query("SELECT * FROM projects WHERE localId = :localId")
    abstract fun observeByLocalId(localId: Long): Flow<ProjectRow?>

    @Query("SELECT * FROM projects ORDER BY title COLLATE NOCASE")
    abstract fun observeAll(): Flow<List<ProjectRow>>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM projects WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM projects WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    /** Every project currently sitting in one of the daily slots. */
    @Query("SELECT * FROM projects WHERE troikiCategory IS NOT NULL")
    abstract suspend fun inTroikiCategories(): List<ProjectRow>

    @Query("SELECT COUNT(*) FROM projects")
    abstract suspend fun count(): Int

    /** How many projects are pinned, which is what the pinned cap is checked against. */
    @Query("SELECT COUNT(*) FROM projects WHERE isPinned = 1")
    abstract suspend fun countPinned(): Int

    /**
     * How many open projects already sit in one of the three daily slots.
     *
     * Only open ones count: a finished project still carries the category it was
     * worked on under, and counting it would leave the slot permanently full.
     */
    @Query("SELECT COUNT(*) FROM projects WHERE troikiCategory = :category AND status = 'open'")
    abstract suspend fun countInTroikiCategory(category: TroikiCategory): Int

    @Transaction
    open suspend fun upsertByServerId(row: ProjectRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)

    // --- labels attached to a project ---

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    abstract suspend fun insertLabels(rows: List<ProjectLabelRow>)

    @Query("DELETE FROM project_labels WHERE projectLocalId = :projectLocalId")
    abstract suspend fun clearLabels(projectLocalId: Long)

    @Query("SELECT labelLocalId FROM project_labels WHERE projectLocalId = :projectLocalId")
    abstract suspend fun labelLocalIds(projectLocalId: Long): List<Long>

    /** Replaces a project's labels with exactly [labelLocalIds]. */
    @Transaction
    open suspend fun setLabels(
        projectLocalId: Long,
        labelLocalIds: Collection<Long>,
    ) {
        clearLabels(projectLocalId)
        insertLabels(labelLocalIds.distinct().map { ProjectLabelRow(projectLocalId, it) })
    }
}
