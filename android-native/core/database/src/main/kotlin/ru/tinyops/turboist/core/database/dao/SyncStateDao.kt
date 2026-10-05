package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.SyncStateRow

/**
 * How far the replica has caught up with the server's change history.
 *
 * Reading `null` means the replica has never synced, which is what sends a fresh
 * install to a full copy instead of to a delta it could not place.
 */
@Dao
interface SyncStateDao {
    @Upsert
    suspend fun save(row: SyncStateRow)

    @Query("SELECT * FROM sync_state LIMIT 1")
    suspend fun get(): SyncStateRow?

    @Query("SELECT * FROM sync_state LIMIT 1")
    fun observe(): Flow<SyncStateRow?>

    @Query("DELETE FROM sync_state")
    suspend fun clear()
}
