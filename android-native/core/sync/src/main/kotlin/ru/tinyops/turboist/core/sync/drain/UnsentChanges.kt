package ru.tinyops.turboist.core.sync.drain

import android.util.Log
import androidx.room.withTransaction
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.sync.write.OutboxOpKind

/**
 * What the server has not been told, in the two states a user can be shown.
 *
 * *Waiting* is the ordinary state: the change is made, the queue holds it, and it
 * will go out. *Set aside* is the state that needs a person — the server refused
 * the change, so it will never go out, and the only thing left to decide is
 * whether the user wants to know or wants it gone.
 *
 * Discarding usually removes the record of the refusal and nothing else: the
 * change never happened on the server, and the row it was made against has
 * already been corrected by a catch-up. The one exception is a refused
 * *creation*, whose row exists on this device and nowhere else — with the write
 * thrown away nothing will ever bring it back, and leaving it would be an item
 * on screen that no catch-up can correct and no later write can send. So it goes
 * with the refusal, in the same transaction, and only while it is still nameless:
 * a row the server did answer for has been replicated since and is real.
 */
class UnsentChanges(private val db: TurboistDatabase) {
    /** Changes still queued, oldest first — the order they will be sent in. */
    fun waiting(): Flow<List<OutboxOpRow>> = db.outbox().observeAll()

    /** How many changes the server has not been told about yet. */
    fun waitingCount(): Flow<Int> = db.outbox().observeUnsentCount()

    /** Changes the server refused, oldest first. */
    fun setAside(): Flow<List<QuarantinedOpRow>> = db.outbox().observeQuarantined()

    /** The same, asked once. */
    suspend fun setAsideNow(): List<QuarantinedOpRow> = db.outbox().quarantined()

    /** Closes one refusal. True when there was one to close. */
    suspend fun discard(id: String): Boolean {
        val discarded =
            db.withTransaction {
                val refused = db.outbox().quarantinedById(id) ?: return@withTransaction false
                db.outbox().discardQuarantined(id)
                dropAbandonedCreation(refused)
                true
            }
        if (discarded) Log.i(DRAIN_LOG_TAG, "The user discarded a change the server had refused")
        return discarded
    }

    /** Closes every refusal at once, and reports how many there were. */
    suspend fun discardAll(): Int {
        val discarded =
            db.withTransaction {
                val refused = db.outbox().quarantined()
                db.outbox().discardAllQuarantined()
                refused.forEach { dropAbandonedCreation(it) }
                refused.size
            }
        if (discarded > 0) Log.i(DRAIN_LOG_TAG, "The user discarded $discarded change(s) the server had refused")
        return discarded
    }

    /**
     * Removes the row a discarded creation was for, when that row is still known
     * to this device alone.
     *
     * A creation the server did answer for has a server id by now — the answer
     * arrived, so the write is not what was refused — and such a row is left
     * exactly where it is.
     */
    private suspend fun dropAbandonedCreation(refused: QuarantinedOpRow) {
        if (OutboxOpKind.fromStored(refused.op)?.creates != true) return
        val localId = refused.entityLocalId
        if (localId == NO_LOCAL_ID) return
        when (refused.entity) {
            ReplicaEntityKind.TASK -> dropTask(localId)
            ReplicaEntityKind.PROJECT -> dropProject(localId)
            ReplicaEntityKind.SECTION -> dropSection(localId)
            ReplicaEntityKind.CONTEXT -> dropContext(localId)
            ReplicaEntityKind.LABEL -> dropLabel(localId)
            ReplicaEntityKind.TASK_TEMPLATE -> dropTemplate(localId)
            // Nothing else is ever brought into being by one of those writes: a
            // relation is made by an op of its own against rows that already
            // exist, and the three settings documents are single rows this device
            // never creates.
            ReplicaEntityKind.TASK_RELATION,
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> Unit
        }
    }

    private suspend fun dropTask(localId: Long) {
        val row = db.tasks().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.tasks().delete(row)
    }

    private suspend fun dropProject(localId: Long) {
        val row = db.projects().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.projects().delete(row)
    }

    private suspend fun dropSection(localId: Long) {
        val row = db.sections().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.sections().delete(row)
    }

    private suspend fun dropContext(localId: Long) {
        val row = db.contexts().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.contexts().delete(row)
    }

    private suspend fun dropLabel(localId: Long) {
        val row = db.labels().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.labels().delete(row)
    }

    private suspend fun dropTemplate(localId: Long) {
        val row = db.taskTemplates().byLocalId(localId) ?: return
        if (row.serverId != null) return
        db.taskTemplates().delete(row)
    }
}
