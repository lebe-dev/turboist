package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.BlockEdge

/**
 * One link on a task, with the peer at the other end named.
 *
 * A screen that lists links has to show what is on the other side of each one,
 * and a title is what a person recognises — an id is not. [outgoing] is the edge
 * read from this task's side: for a `blocks` edge it says whether this task is
 * the one holding the other up.
 */
data class TaskRelationPeer(
    val relationLocalId: Long,
    val type: RelationType,
    val outgoing: Boolean,
    val peerLocalId: Long,
    val peerServerId: Long?,
    val peerTitle: String,
    val peerStatus: TaskStatus,
)

@Dao
abstract class TaskRelationDao {
    @Insert
    abstract suspend fun insert(row: TaskRelationRow): Long

    @Update
    abstract suspend fun update(row: TaskRelationRow)

    @Delete
    abstract suspend fun delete(row: TaskRelationRow)

    @Query("SELECT localId FROM task_relations WHERE serverId = :serverId")
    abstract suspend fun localIdForServerId(serverId: Long): Long?

    /**
     * Writes the server id onto a row created on this device.
     *
     * This is the moment a locally created record becomes addressable: until it
     * runs, the row exists only here and nothing can name it to the server. The
     * local id is untouched, so every reference already pointing at the row —
     * on screen and in the queue of unsent writes — still points at it.
     */
    @Query("UPDATE task_relations SET serverId = :serverId WHERE localId = :localId")
    abstract suspend fun assignServerId(
        localId: Long,
        serverId: Long,
    )

    @Query("SELECT * FROM task_relations WHERE localId = :localId")
    abstract suspend fun byLocalId(localId: Long): TaskRelationRow?

    /** Every edge one task is an endpoint of, in either direction. */
    @Query(
        "SELECT * FROM task_relations " +
            "WHERE sourceTaskLocalId = :taskLocalId OR targetTaskLocalId = :taskLocalId " +
            "ORDER BY createdAt, localId",
    )
    abstract fun observeForTask(taskLocalId: Long): Flow<List<TaskRelationRow>>

    @Query(
        "SELECT * FROM task_relations " +
            "WHERE sourceTaskLocalId = :taskLocalId OR targetTaskLocalId = :taskLocalId " +
            "ORDER BY createdAt, localId",
    )
    abstract suspend fun forTask(taskLocalId: Long): List<TaskRelationRow>

    /**
     * Every server id the replica currently holds for this table.
     *
     * A complete seed names every row the server has; anything here that the seed
     * does not name no longer exists there. Rows created on this device carry no
     * server id and are absent from this answer by construction, which is what
     * keeps them out of that comparison.
     */
    @Query("SELECT serverId FROM task_relations WHERE serverId IS NOT NULL")
    abstract suspend fun knownServerIds(): List<Long>

    @Query("DELETE FROM task_relations WHERE serverId = :serverId")
    abstract suspend fun deleteByServerId(serverId: Long): Int

    @Query("SELECT COUNT(*) FROM task_relations")
    abstract suspend fun count(): Int

    /**
     * Every link one task is an end of, with the task at the other end named.
     *
     * Two arms rather than one join with an `OR`, because each arm knows which
     * end is the peer — which is what makes "does this task hold the other up"
     * answerable in the query instead of in a second pass over the rows.
     *
     * The order is the order the links were made in, so a list does not reshuffle
     * itself when an unrelated one is added.
     */
    @Query(
        "SELECT r.localId AS relationLocalId, r.type AS type, 1 AS outgoing, " +
            "p.localId AS peerLocalId, p.serverId AS peerServerId, p.title AS peerTitle, " +
            "p.status AS peerStatus, r.createdAt AS relationCreatedAt " +
            "FROM task_relations r JOIN tasks p ON p.localId = r.targetTaskLocalId " +
            "WHERE r.sourceTaskLocalId = :taskLocalId " +
            "UNION ALL " +
            "SELECT r.localId AS relationLocalId, r.type AS type, 0 AS outgoing, " +
            "p.localId AS peerLocalId, p.serverId AS peerServerId, p.title AS peerTitle, " +
            "p.status AS peerStatus, r.createdAt AS relationCreatedAt " +
            "FROM task_relations r JOIN tasks p ON p.localId = r.sourceTaskLocalId " +
            "WHERE r.targetTaskLocalId = :taskLocalId " +
            "ORDER BY relationCreatedAt, relationLocalId",
    )
    abstract fun observePeersForTask(taskLocalId: Long): Flow<List<TaskRelationPeer>>

    /**
     * The link already recorded for exactly this pair and kind, if there is one.
     *
     * The store refuses a second copy of the same link, so this is what lets the
     * device say so at the moment the user asks for it rather than letting the
     * request go out and come back rejected.
     */
    @Query(
        "SELECT localId FROM task_relations " +
            "WHERE sourceTaskLocalId = :sourceTaskLocalId AND targetTaskLocalId = :targetTaskLocalId " +
            "AND type = :type",
    )
    abstract suspend fun localIdForEdge(
        sourceTaskLocalId: Long,
        targetTaskLocalId: Long,
        type: RelationType,
    ): Long?

    /**
     * Every `blocks` edge in the workspace, whatever state its ends are in.
     *
     * This is the graph the loop check reads. A finished blocker is still an edge:
     * it has released what it was holding up, but reopening it would bring the
     * wait back, so a loop drawn through it is a loop.
     */
    @Query(
        "SELECT sourceTaskLocalId AS blockerLocalId, targetTaskLocalId AS blockedLocalId " +
            "FROM task_relations WHERE type = 'blocks'",
    )
    abstract suspend fun blockEdges(): List<BlockEdge>

    /**
     * Every `blocks` edge that still holds something up.
     *
     * Only the ones whose blocker is still open: a blocker that was completed —
     * or cancelled, which settles it just as finally — no longer stands in
     * anything's way.
     */
    @Query(
        "SELECT r.sourceTaskLocalId AS blockerLocalId, r.targetTaskLocalId AS blockedLocalId " +
            "FROM task_relations r JOIN tasks s ON s.localId = r.sourceTaskLocalId " +
            "WHERE r.type = 'blocks' AND s.status = 'open'",
    )
    abstract suspend fun openBlockEdges(): List<BlockEdge>

    @Transaction
    open suspend fun upsertByServerId(row: TaskRelationRow): Long =
        upsertResolvingServerId(row, ::localIdForServerId, ::insert, ::update)
}
