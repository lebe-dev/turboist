package ru.tinyops.turboist.core.database.entity

import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind

/**
 * How far the replica has caught up.
 *
 * [cursor] is the position in the server's change history that has been fully
 * applied. It is written in the same transaction as the changes it accounts for,
 * so a process killed halfway through a page resumes from the last page that
 * actually landed rather than skipping one.
 *
 * [epoch] guards against a history that was replaced rather than extended — a
 * restored backup rewrites it wholesale. A cursor is only meaningful within the
 * epoch it was taken in; presented under a newer one it is refused, and the
 * replica starts again from a full copy instead of quietly diverging.
 *
 * The absence of the row means "never synced", which is a real and expected
 * state on first launch — it is not seeded with zeroes, because a zero cursor
 * under an invented epoch would claim a position in a history the device has
 * never seen.
 */
@Entity(tableName = "sync_state")
data class SyncStateRow(
    @PrimaryKey val id: Int = SINGLE_ROW_ID,
    val epoch: Long,
    val cursor: Long,
    val lastSyncAt: Long? = null,
)

/**
 * One write waiting to reach the server.
 *
 * [id] is a UUID generated when the op is queued, and it is also the key the
 * request is sent under, so a retry after a lost answer is recognised as the
 * same write rather than performed twice. Generating it at queue time rather
 * than at send time is what makes that true across a process death.
 *
 * [seq] is the queue order. It is a separate column rather than an ordering by
 * time because a bulk action queues several ops within the same millisecond and
 * their order among themselves is exactly what must not be lost.
 *
 * [payload] describes the write and [op] names which write it is; the catalog of
 * op names, and the shape of what they carry, belong to the layer that builds and
 * sends them, so this table stores whatever it is given. What that layer stores
 * is not quite the request: the values are already in the form the server reads,
 * but references to other rows are the device's own ids, because a row created
 * offline has no server id yet and a reference recorded as "nothing yet" could
 * never be repaired. What the replica does own is
 * [entity] plus [entityLocalId]: the row the op will change, which is what makes
 * "is there an unsent write against this row" answerable.
 */
@Entity(
    tableName = "outbox",
    indices = [
        Index(value = ["seq"], unique = true),
        Index(value = ["entity", "entityLocalId"]),
        Index(value = ["state"]),
    ],
)
data class OutboxOpRow(
    @PrimaryKey val id: String,
    val seq: Long = UNASSIGNED_SEQ,
    val op: String,
    val payload: String,
    val entity: ReplicaEntityKind,
    val entityLocalId: Long,
    val state: OutboxState = OutboxState.PENDING,
    val attempts: Int = 0,
    val lastError: String? = null,
    val createdAt: Long,
    val updatedAt: Long,
) {
    companion object {
        /**
         * The queue position of an op that has not been enqueued yet. Enqueuing
         * assigns the real one, so a caller never has to know the queue's tail.
         */
        const val UNASSIGNED_SEQ: Long = 0L
    }
}

/**
 * A write the server refused, kept instead of thrown away.
 *
 * A refusal is not a transport problem to retry — the server has answered, and
 * the answer is no. Retrying would stall the queue behind an op that can never
 * land, so the op leaves the queue and the ops behind it carry on. It is kept
 * because the user made a change and is entitled to be told it did not happen,
 * with the payload intact so they can see what it was.
 *
 * [errorCode] is the server's own code for the refusal where it gave one, which
 * is what lets the screen explain the reason rather than show a status number.
 *
 * [blockedBy] holds the ids of the still-open tasks that stood in the way when
 * the refusal was "this task is blocked". They are kept because the code and the
 * message alone cannot answer the only question the user has — *blocked by
 * what?* — the message is the same fixed sentence for every such refusal, and by
 * the time the list is read the server's answer is long gone. Server ids, since
 * that is what the server named and what the replica can resolve a row by. The
 * column is a plain comma-separated list rather than a nested document: it holds
 * numbers, it is never queried by, and a bookkeeping table is a poor place to
 * grow a second serialisation format.
 */
@Entity(
    tableName = "quarantine",
    indices = [
        Index(value = ["entity", "entityLocalId"]),
        Index(value = ["quarantinedAt"]),
    ],
)
data class QuarantinedOpRow(
    @PrimaryKey val id: String,
    val op: String,
    val payload: String,
    val entity: ReplicaEntityKind,
    val entityLocalId: Long,
    val errorCode: String,
    val errorMessage: String = "",
    val httpStatus: Int? = null,
    val blockedBy: String = "",
    val attempts: Int = 0,
    val createdAt: Long,
    val quarantinedAt: Long,
)

/**
 * The ids [QuarantinedOpRow.blockedBy] names, as numbers.
 *
 * An extension rather than a property on the row so the stored column stays the
 * one thing the table has, and so nothing has to decide whether a derived value
 * belongs in the schema.
 */
val QuarantinedOpRow.blockerServerIds: List<Long>
    get() = decodeBlockers(blockedBy)

/** How a list of blocker ids is written into the column, and read back out of it. */
internal fun encodeBlockers(ids: List<Long>): String = ids.joinToString(separator = ",")

internal fun decodeBlockers(stored: String): List<Long> =
    if (stored.isEmpty()) emptyList() else stored.split(',').mapNotNull(String::toLongOrNull)
