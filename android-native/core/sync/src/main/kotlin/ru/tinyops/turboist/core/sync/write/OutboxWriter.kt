package ru.tinyops.turboist.core.sync.write

import androidx.room.withTransaction
import kotlinx.serialization.json.Json
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.network.TurboistJson
import java.util.UUID

/**
 * How an op is stored in, and read back out of, the queue.
 *
 * The name of the op is written inside the payload as well as in its own column.
 * That is not redundancy for its own sake: the column is what a query can filter
 * and group by, and the field inside is what makes a payload readable on its own
 * — a row copied out of the database for a bug report says what it is without
 * needing the row it came from.
 */
object OutboxOpCodec {
    /**
     * The queue's own JSON, which differs from the wire's in two ways.
     *
     * It names the discriminator `op`, matching the column, so the two forms of
     * the same fact are spelled the same.
     *
     * And it writes nulls. A payload has to survive being *read back*, and the
     * wire's habit of dropping nulls loses information on the way in: a field
     * whose stored value is null but whose default is not would come back as the
     * default. Nothing grows by this — a field left at its default is still
     * omitted — and stored writes stay recognisable as what they were.
     */
    private val json: Json =
        Json(TurboistJson) {
            classDiscriminator = "op"
            explicitNulls = true
        }

    fun encode(op: OutboxOp): String = json.encodeToString(OutboxOp.serializer(), op)

    /**
     * The op a stored payload denotes.
     *
     * @throws IllegalArgumentException when this build cannot read the payload —
     *   which is a real possibility after a downgrade, and is why the caller
     *   surfaces such a row instead of sending it.
     */
    fun decode(payload: String): OutboxOp = json.decodeFromString(OutboxOp.serializer(), payload)
}

/** The clock the write path reads. Injected so a test can state the moment rather than observe it. */
fun interface WriteClock {
    fun now(): Long

    companion object {
        val System: WriteClock = WriteClock { java.lang.System.currentTimeMillis() }
    }
}

/**
 * Where an op's identity comes from.
 *
 * The identity is minted when the write is *queued*, not when it is sent, and
 * that is the whole point: it travels with the request as the key the server
 * recognises a repeat by, so an answer lost to a dropped connection — or to the
 * app being killed between sending and hearing back — is safe to ask for again.
 * A key minted at send time would be a different key on the second attempt and
 * the write would happen twice.
 */
fun interface OpIdFactory {
    fun next(): String

    companion object {
        val Uuid: OpIdFactory = OpIdFactory { UUID.randomUUID().toString() }
    }
}

/**
 * What a completed write left behind: the queued op, and the row it changed.
 *
 * [entityLocalId] is the caller's answer to "what did I just create or edit" —
 * for a create it is a row that exists only on this device so far, which is
 * exactly what a screen needs to navigate to it.
 */
data class QueuedWrite(
    val opId: String,
    val entityLocalId: Long,
)

/**
 * The one place a write is made durable.
 *
 * Every user action runs through [transaction]: the optimistic change to the
 * replica and the op that will tell the server about it are written together or
 * not at all. Split into two steps they would sometimes be half-done — a screen
 * showing a change nobody will ever hear about, or a request for a change the
 * screen never made.
 */
class OutboxWriter(
    private val db: TurboistDatabase,
    private val clock: WriteClock = WriteClock.System,
    private val opIds: OpIdFactory = OpIdFactory.Uuid,
) {
    /** The moment the write is being made, for both the replica row and the op. */
    fun now(): Long = clock.now()

    /** Runs [body] as one transaction, so the optimistic change and its op commit together. */
    suspend fun <T> transaction(body: suspend () -> T): T = db.withTransaction { body() }

    /**
     * Queues an op against the row it changes.
     *
     * [entityLocalId] is left at [NO_LOCAL_ID] by the ops that change more than
     * one row — a bulk action, a preference document, the daily plan. No single
     * row can stand for those, and claiming one would make exactly one of the
     * rows they touch look protected from an incoming change while the rest were
     * not. They are unprotected together instead, which is honest: the queue is
     * drained before the next page is pulled, so the server's answer already
     * includes them by the time one arrives.
     */
    suspend fun enqueue(
        op: OutboxOp,
        entity: ReplicaEntityKind,
        entityLocalId: Long = NO_LOCAL_ID,
    ): String {
        val id = opIds.next()
        val at = clock.now()
        db.outbox().enqueue(
            OutboxOpRow(
                id = id,
                op = op.kind.stored,
                payload = OutboxOpCodec.encode(op),
                entity = entity,
                entityLocalId = entityLocalId,
                createdAt = at,
                updatedAt = at,
            ),
        )
        return id
    }
}
