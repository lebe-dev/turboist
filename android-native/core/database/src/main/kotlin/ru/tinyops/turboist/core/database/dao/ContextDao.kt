package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.ContextRow

@Dao
abstract class ContextDao {
    @Insert
    abstract suspend fun insert(row: ContextRow): Long

    @Update
    abstract suspend fun update(row: ContextRow)

    @Delete
    abstract suspend fun delete(row: ContextRow)

    @Query("SELECT localId FROM contexts WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE contexts SET serverId = :serverId, updatedAt = :updatedAt WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
        updatedAt: Long,
    )

    @Query("SELECT * FROM contexts WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): ContextRow?

    @Query("SELECT * FROM contexts WHERE serverId = :serverId")
    abstract suspend fun byServerId(serverId: Long): ContextRow?

    /** One context, and again whenever it changes. */
    @Query("SELECT * FROM contexts WHERE localId = :localId")
    abstract fun observeByLocalId(localId: Long): Flow<ContextRow?>

    @Query("SELECT * FROM contexts ORDER BY name COLLATE NOCASE")
    abstract fun observeAll(): Flow<List<ContextRow>>

    /** Removes the row standing for [serverId]; answers how many rows that was. */
    @Query("DELETE FROM contexts WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM contexts WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("SELECT COUNT(*) FROM contexts")
    abstract suspend fun count(): Int

    /** Applies an incoming record, keeping the local id of the row it names. */
    @Transaction
    open suspend fun upsertByServerId(row: ContextRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)
}
