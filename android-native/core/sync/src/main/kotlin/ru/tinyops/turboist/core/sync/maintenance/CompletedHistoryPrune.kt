package ru.tinyops.turboist.core.sync.maintenance

import android.util.Log
import androidx.room.withTransaction
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.view.ViewWindows
import java.time.Clock

/**
 * How much finished work the device keeps a copy of.
 *
 * It is the same stretch the server seeds a new replica with and keeps its change
 * history for, and the two have to be the same number: a device is handed exactly
 * the history it can still be told about changes to, so anything beyond it can
 * only ever go stale in place. Older history is not lost — it stays on the server
 * and is read on demand — it simply does not live here.
 */
const val REPLICATED_COMPLETED_HISTORY_DAYS: Int = 90

/**
 * Drops the finished work that has aged out of the replica's window.
 *
 * Without this a device accumulates completions forever. The window is not a
 * quota, it is what the device and the server agree the device holds: a
 * completion older than the window is one no future change page will ever mention
 * again, so keeping it means keeping a row that can no longer be corrected and
 * that every history query has to read past.
 *
 * Two rules keep it from ever costing anything:
 *
 * - **A row with an unsent write against it is never touched.** It carries a
 *   change the server has not been told about, and deleting it would delete the
 *   user's own work between them making it and it being sent.
 * - **A task is only dropped when everything beneath it goes too.** Removing a
 *   task removes its subtasks with it, so a long-finished parent with a subtask
 *   that is still open, still dirty, or still inside the window is left where it
 *   is. Losing an open task because its parent was finished a season ago would be
 *   losing live work.
 */
class CompletedHistoryPrune(
    private val db: TurboistDatabase,
    private val clock: Clock,
    private val days: Int = REPLICATED_COMPLETED_HISTORY_DAYS,
) : ReplicaMaintenance {
    override suspend fun runMaintenance() {
        prune()
    }

    /**
     * Removes what has aged out and answers with how many rows went.
     *
     * The count is every task that left, subtasks included. It is taken from the
     * set that was decided on rather than from what the delete reported, because
     * the two differ: a subtask goes because the foreign key takes it with its
     * parent, and a cascade is not counted among a statement's own changes. The
     * set is nonetheless exact — a subtask that had to stay would have kept its
     * parent, so everything beneath a task being removed is in the set already.
     */
    suspend fun prune(): Int {
        val cutoff = ViewWindows.completedHistory(clock.instant(), clock.zone, days).from
        val aged = db.tasks().completedBefore(cutoff)
        if (aged.isEmpty()) return 0

        val candidates = aged.toMutableSet()
        for (batch in aged.chunked(ID_BATCH)) {
            candidates -= db.outbox().dirtyLocalIds(ReplicaEntityKind.TASK, batch)
        }
        if (candidates.isEmpty()) return 0

        val removable = candidates - ancestorsOfSurvivors(candidates)
        if (removable.isEmpty()) return 0

        db.withTransaction {
            for (batch in removable.chunked(ID_BATCH)) {
                db.tasks().deleteByLocalIds(batch)
            }
        }
        Log.i(
            MAINTENANCE_LOG_TAG,
            "Removed ${removable.size} completed tasks that aged out of the replica's history window",
        )
        return removable.size
    }

    /**
     * The candidates that have something under them which is staying.
     *
     * Disqualification travels upwards: a candidate kept because of a child is
     * itself a survivor, so whichever candidate stands above it has to be kept as
     * well. Walking the chain is what stops a whole branch being taken out from
     * under one row that had to stay.
     */
    private suspend fun ancestorsOfSurvivors(candidates: Set<Long>): Set<Long> {
        val children = candidates.chunked(ID_BATCH).flatMap { db.tasks().childrenOf(it) }
        if (children.isEmpty()) return emptySet()
        val parentOf = children.associate { it.taskLocalId to it.parentLocalId }
        val keep = mutableSetOf<Long>()
        val pending = ArrayDeque(children.filter { it.taskLocalId !in candidates }.map { it.parentLocalId })
        while (pending.isNotEmpty()) {
            val id = pending.removeFirst()
            if (!keep.add(id)) continue
            parentOf[id]?.let(pending::addLast)
        }
        return keep
    }

    private companion object {
        /**
         * How many ids go into one statement. SQLite caps how many values a
         * single statement may bind, and a device that has been in use for years
         * can age out more completions than that in one go.
         */
        const val ID_BATCH = 400
    }
}

internal const val MAINTENANCE_LOG_TAG = "TurboistSync"
