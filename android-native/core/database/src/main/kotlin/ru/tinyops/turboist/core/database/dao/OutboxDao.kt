package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.database.entity.encodeBlockers
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind

/**
 * The queue of writes waiting to reach the server, and the ops it refused.
 *
 * Reading is always by queue position, never by time: an op's place in line is a
 * correctness property, since a move applied after the completion it preceded
 * produces a different result than the user asked for.
 */
@Dao
abstract class OutboxDao {
    @Insert
    abstract suspend fun insert(row: OutboxOpRow)

    @Update
    abstract suspend fun update(row: OutboxOpRow)

    @Query("SELECT COALESCE(MAX(seq), 0) FROM outbox")
    abstract suspend fun lastSeq(): Long

    /**
     * Queues a write and gives it the next place in line.
     *
     * The position is assigned here rather than by the caller so that two
     * actions queued in the same millisecond still have an order, and so that
     * the caller never has to know the tail of the queue.
     */
    @Transaction
    open suspend fun enqueue(op: OutboxOpRow): OutboxOpRow {
        val queued = op.copy(seq = lastSeq() + 1)
        insert(queued)
        return queued
    }

    @Query("SELECT * FROM outbox WHERE id = :id")
    abstract suspend fun byId(id: String): OutboxOpRow?

    /**
     * The head of the queue — the op being sent, or the next one to be.
     *
     * There is no "next unsent" query, because there is no such thing: exactly
     * one op is ever in flight and nothing behind it may overtake it, so the
     * head is always the op the drainer is concerned with.
     */
    @Query("SELECT * FROM outbox ORDER BY seq LIMIT 1")
    abstract suspend fun head(): OutboxOpRow?

    @Query("SELECT * FROM outbox ORDER BY seq")
    abstract suspend fun all(): List<OutboxOpRow>

    @Query("SELECT * FROM outbox ORDER BY seq")
    abstract fun observeAll(): Flow<List<OutboxOpRow>>

    /** How many writes the server has not been told about yet. */
    @Query("SELECT COUNT(*) FROM outbox")
    abstract fun observeUnsentCount(): Flow<Int>

    /** The same count, asked once — what a drain reports as still waiting. */
    @Query("SELECT COUNT(*) FROM outbox")
    abstract suspend fun count(): Int

    @Query("DELETE FROM outbox WHERE id = :id")
    abstract suspend fun deleteById(id: String): Int

    @Query("DELETE FROM outbox")
    abstract suspend fun clear()

    // --- dirty rows ---

    @Query(
        "SELECT EXISTS(SELECT 1 FROM outbox WHERE entity = :entity " +
            "AND entityLocalId = :localId AND state IN (:states))",
    )
    abstract suspend fun isReferenced(
        entity: ReplicaEntityKind,
        localId: Long,
        states: Collection<OutboxState>,
    ): Boolean

    @Query(
        "SELECT DISTINCT entityLocalId FROM outbox WHERE entity = :entity " +
            "AND entityLocalId IN (:localIds) AND state IN (:states)",
    )
    abstract suspend fun referencedLocalIds(
        entity: ReplicaEntityKind,
        localIds: Collection<Long>,
        states: Collection<OutboxState>,
    ): List<Long>

    /**
     * Whether an unsent write is queued against this row.
     *
     * Such a row is *dirty*, and an incoming record must not be written over it:
     * the user is looking at a change the server has not been told about yet, and
     * overwriting it would make their edit vanish and then reappear. The op's own
     * send, and the pull that follows it, bring the row back to server truth.
     */
    suspend fun isDirty(
        entity: ReplicaEntityKind,
        localId: Long,
    ): Boolean = isReferenced(entity, localId, OutboxState.BLOCKING)

    /** The same question asked for a whole page of incoming records at once. */
    suspend fun dirtyLocalIds(
        entity: ReplicaEntityKind,
        localIds: Collection<Long>,
    ): Set<Long> {
        if (localIds.isEmpty()) return emptySet()
        return referencedLocalIds(entity, localIds, OutboxState.BLOCKING).toSet()
    }

    /**
     * The writes queued against one row, in queue order.
     *
     * Asked when the row itself is gone: the server says it was deleted, so every
     * op still waiting to change it can never land and has to leave the queue.
     */
    @Query("SELECT * FROM outbox WHERE entity = :entity AND entityLocalId = :localId ORDER BY seq")
    abstract suspend fun opsFor(
        entity: ReplicaEntityKind,
        localId: Long,
    ): List<OutboxOpRow>

    // --- quarantine ---

    @Insert
    abstract suspend fun insertQuarantined(row: QuarantinedOpRow)

    @Query("SELECT * FROM quarantine ORDER BY quarantinedAt, id")
    abstract fun observeQuarantined(): Flow<List<QuarantinedOpRow>>

    @Query("SELECT * FROM quarantine ORDER BY quarantinedAt, id")
    abstract suspend fun quarantined(): List<QuarantinedOpRow>

    /** One refused write, read before it is closed so the caller knows what it was. */
    @Query("SELECT * FROM quarantine WHERE id = :id")
    abstract suspend fun quarantinedById(id: String): QuarantinedOpRow?

    @Query("DELETE FROM quarantine WHERE id = :id")
    abstract suspend fun discardQuarantined(id: String): Int

    /** Clears every refused write at once, for the user who wants the list gone rather than read. */
    @Query("DELETE FROM quarantine")
    abstract suspend fun discardAllQuarantined(): Int

    /**
     * Takes a refused write out of the queue and keeps it where the user can see
     * it.
     *
     * Leaving it in the queue would stall every op behind it on an answer that
     * will never change; dropping it would lose a change the user made without
     * telling them. One transaction, so the op is never in both places or in
     * neither.
     *
     * [blockedBy] carries the ids the server named when it refused a completion
     * because something still blocks the task. It is kept with the row because
     * the refusal's message is the same sentence every time and the ids are the
     * only part that says which tasks to go and look at.
     */
    @Transaction
    open suspend fun quarantine(
        op: OutboxOpRow,
        errorCode: String,
        errorMessage: String,
        httpStatus: Int?,
        quarantinedAt: Long,
        blockedBy: List<Long> = emptyList(),
    ) {
        deleteById(op.id)
        insertQuarantined(
            QuarantinedOpRow(
                id = op.id,
                op = op.op,
                payload = op.payload,
                entity = op.entity,
                entityLocalId = op.entityLocalId,
                errorCode = errorCode,
                errorMessage = errorMessage,
                httpStatus = httpStatus,
                blockedBy = encodeBlockers(blockedBy),
                attempts = op.attempts,
                createdAt = op.createdAt,
                quarantinedAt = quarantinedAt,
            ),
        )
    }
}
