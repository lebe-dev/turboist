package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow

@Dao
abstract class ProjectSectionDao {
    @Insert
    abstract suspend fun insert(row: ProjectSectionRow): Long

    @Update
    abstract suspend fun update(row: ProjectSectionRow)

    @Delete
    abstract suspend fun delete(row: ProjectSectionRow)

    @Query("SELECT localId FROM project_sections WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE project_sections SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    @Query("SELECT * FROM project_sections WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): ProjectSectionRow?

    @Query(
        "SELECT * FROM project_sections WHERE projectLocalId = :projectLocalId " +
            "ORDER BY position, localId",
    )
    abstract fun observeForProject(projectLocalId: Long): Flow<List<ProjectSectionRow>>

    /**
     * Every column of every board, grouped by the project it belongs to.
     *
     * A screen that offers "move this task somewhere" has to show the columns of
     * projects it has not opened, so asking per project would mean one standing
     * query per row of the picker. A workspace holds tens of sections, so the
     * whole set is cheaper than the bookkeeping of slicing it.
     */
    @Query("SELECT * FROM project_sections ORDER BY projectLocalId, position, localId")
    abstract fun observeAll(): Flow<List<ProjectSectionRow>>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM project_sections WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM project_sections WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    /**
     * The rightmost position on a board, or -1 for an empty one, so a new column
     * lands after the ones already there. The server assigns the real position
     * and its answer wins on the next pull; this is only what the board looks
     * like in the meantime.
     */
    @Query("SELECT COALESCE(MAX(position), -1) FROM project_sections WHERE projectLocalId = :projectLocalId")
    abstract suspend fun lastPosition(projectLocalId: Long): Int

    /**
     * The columns of one board in the order they are drawn, read once.
     *
     * The position a column ends up at after a move is decided from the whole
     * board rather than from the column alone, so the write path needs the row
     * set as it stands at that instant — not a stream of it. The tie-break on
     * the device's own id matches the one the server falls back on, so two
     * columns that share a position are ordered the same way on both sides.
     */
    @Query(
        "SELECT * FROM project_sections WHERE projectLocalId = :projectLocalId " +
            "ORDER BY position, localId",
    )
    abstract suspend fun forProject(projectLocalId: Long): List<ProjectSectionRow>

    @Query("SELECT COUNT(*) FROM project_sections")
    abstract suspend fun count(): Int

    @Transaction
    open suspend fun upsertByServerId(row: ProjectSectionRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)
}
