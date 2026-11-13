package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import ru.tinyops.turboist.core.database.entity.TextEmbeddingRow

/** The cache of embeddings label suggestions keep beside the replica. */
@Dao
interface TextEmbeddingDao {
    @Query("SELECT * FROM text_embeddings")
    suspend fun embeddings(): List<TextEmbeddingRow>

    @Query("SELECT `key` FROM text_embeddings")
    suspend fun embeddingKeys(): List<String>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun putEmbeddings(rows: List<TextEmbeddingRow>)

    @Query("DELETE FROM text_embeddings WHERE `key` IN (:keys)")
    suspend fun deleteEmbeddings(keys: List<String>)

    @Query("DELETE FROM text_embeddings")
    suspend fun clearEmbeddings()
}
