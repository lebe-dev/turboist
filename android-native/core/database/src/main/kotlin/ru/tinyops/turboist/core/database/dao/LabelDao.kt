package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.LabelRow

@Dao
abstract class LabelDao {
    @Insert
    abstract suspend fun insert(row: LabelRow): Long

    @Update
    abstract suspend fun update(row: LabelRow)

    @Delete
    abstract suspend fun delete(row: LabelRow)

    @Query("SELECT localId FROM labels WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE labels SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    @Query("SELECT * FROM labels WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): LabelRow?

    @Query("SELECT * FROM labels WHERE serverId = :serverId")
    abstract suspend fun byServerId(serverId: Long): LabelRow?

    /**
     * The label with this name, matched the way the server matches it.
     *
     * Names are how the task endpoints refer to labels, so a write made on the
     * device has to resolve one before it can attach it optimistically. A name
     * that resolves to nothing is left for the server to answer: labels are
     * never invented on this side.
     */
    @Query("SELECT * FROM labels WHERE name = :name LIMIT 1")
    abstract suspend fun byName(name: String): LabelRow?

    /** The device ids of the labels the settings payloads name by server id. */
    @Query("SELECT localId FROM labels WHERE serverId IN (:serverIds)")
    abstract suspend fun localIdsForServerIds(serverIds: Collection<Long>): List<Long>

    /**
     * One label, and again whenever it changes.
     *
     * The screen showing a single label is a standing query like every list,
     * so renaming it or recolouring it — here or on another device — redraws
     * the screen, and deleting it answers null, which is how that screen
     * learns it has nothing left to show.
     */
    @Query("SELECT * FROM labels WHERE localId = :localId")
    abstract fun observeByLocalId(localId: Long): Flow<LabelRow?>

    @Query("SELECT * FROM labels ORDER BY name COLLATE NOCASE")
    abstract fun observeAll(): Flow<List<LabelRow>>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM labels WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM labels WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    @Query("SELECT COUNT(*) FROM labels")
    abstract suspend fun count(): Int

    @Transaction
    open suspend fun upsertByServerId(row: LabelRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)
}
