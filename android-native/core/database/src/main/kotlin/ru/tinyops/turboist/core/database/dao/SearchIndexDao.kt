package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query

/**
 * The seam onto the full-text index: which rows match a term.
 *
 * These answer in local ids rather than in rows on purpose. A search screen
 * wants ranked identity first and full rows second, and keeping the index
 * queries free of the shape of a result row means a richer search can be built
 * on top without reopening the index itself.
 *
 * The term must already be a valid full-text query. Building one from what a
 * person typed — escaping, prefix matching, deciding what a space between two
 * words means — is a product decision and belongs above this layer, not in a
 * query string.
 */
@Dao
interface SearchIndexDao {
    @Query("SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH :query")
    suspend fun matchingTaskLocalIds(query: String): List<Long>

    @Query("SELECT rowid FROM projects_fts WHERE projects_fts MATCH :query")
    suspend fun matchingProjectLocalIds(query: String): List<Long>

    @Query("SELECT rowid FROM labels_fts WHERE labels_fts MATCH :query")
    suspend fun matchingLabelLocalIds(query: String): List<Long>

    @Query("SELECT rowid FROM contexts_fts WHERE contexts_fts MATCH :query")
    suspend fun matchingContextLocalIds(query: String): List<Long>
}
