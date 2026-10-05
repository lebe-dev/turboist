package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind

/**
 * Starting and resetting the daily plan.
 *
 * The plan's capacities are counters the server keeps and the replica does not
 * carry, so starting a cycle changes nothing visible here and simply asks. A
 * reset is different: it always empties the three slots, and that *is* visible,
 * so the projects are taken out of them straight away rather than staying on
 * screen in slots the user has just cleared.
 */
class TroikiWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
) {
    /** Begins a cycle. What it changes is the server's counters, which arrive on the next pull. */
    suspend fun start(): QueuedWrite =
        writer.transaction {
            val opId = writer.enqueue(StartTroikiOp, ReplicaEntityKind.PROJECT)
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    /** Ends a cycle and takes every project back out of its slot. */
    suspend fun reset(): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            for (project in db.projects().inTroikiCategories()) {
                db.projects().update(project.copy(troikiCategory = null, updatedAt = at))
            }
            val opId = writer.enqueue(ResetTroikiOp, ReplicaEntityKind.PROJECT)
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    private companion object {
        /** The local id an op carries when it changes more than one row, or none. */
        const val NO_TARGET_ROW: Long = 0L
    }
}
