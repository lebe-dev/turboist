package ru.tinyops.turboist.core.sync

import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.sync.drain.DrainResult
import ru.tinyops.turboist.core.sync.drain.OutboxDrainer
import ru.tinyops.turboist.core.sync.pull.PullResult
import ru.tinyops.turboist.core.sync.pull.SyncPuller

/**
 * What one run of the cycle achieved, in the only three shapes anything outside
 * the engine cares about.
 */
sealed interface SyncCycleResult {
    /** The replica and the server agree, as of a moment ago. */
    data object Synced : SyncCycleResult

    /** Nothing reached the server. Local data is untouched and the cycle is worth trying again. */
    data object Offline : SyncCycleResult

    /**
     * The server was reached and refused. Trying the identical request again
     * would be refused identically, so the next scheduled run is soon enough.
     */
    data class Refused(val cause: Throwable) : SyncCycleResult
}

/**
 * One full turn of the sync engine: send what is queued, then read what changed.
 *
 * Everything that decides *when* to sync — an event from the server, a network
 * that came back, the app coming to the front, the periodic background job, a
 * pull-to-refresh — goes through this one call and knows nothing else about the
 * engine. That is the point of the interface: the trigger layer stays a set of
 * reasons to sync, and the engine stays the definition of what syncing is.
 *
 * The order inside a cycle is fixed and is not a detail. Sending first keeps the
 * server from answering with state that predates this device's own queued
 * writes; reading afterwards picks up everything those writes cascaded into.
 */
fun interface SyncCycle {
    suspend fun runSyncCycle(): SyncCycleResult

    companion object {
        /**
         * The cycle as the app runs it: send what is queued, then read what
         * changed.
         *
         * The read happens only when the queue emptied, and that is a correctness
         * rule rather than an economy. A row created here and not yet sent has no
         * id the server would recognise, so a read taken while its creation is
         * still queued would bring the server's own copy back as something new
         * and the device would hold the same thing twice. Sending first is what
         * makes the two identifiable as one.
         *
         * So a queue that stopped ends the turn, and what stopped it decides how
         * the turn reads: nothing reached the server, or the server refused. Both
         * leave the replica exactly as it was, which is the state every screen is
         * already reading, and the next turn tries again.
         */
        fun sending(
            drainer: OutboxDrainer,
            puller: SyncPuller,
        ): SyncCycle =
            SyncCycle {
                when (val drain = drainer.drain()) {
                    is DrainResult.Drained -> puller.pull().asCycleResult()
                    is DrainResult.NeedsCredentials -> SyncCycleResult.Refused(drain.cause)
                    is DrainResult.Stalled ->
                        if (drain.cause is ApiException.Network) {
                            SyncCycleResult.Offline
                        } else {
                            SyncCycleResult.Refused(drain.cause)
                        }
                }
            }
    }
}

/** Reads a catch-up's answer as a cycle's answer. */
fun PullResult.asCycleResult(): SyncCycleResult =
    when (this) {
        is PullResult.Applied -> SyncCycleResult.Synced
        is PullResult.Offline -> SyncCycleResult.Offline
        is PullResult.Refused -> SyncCycleResult.Refused(cause)
    }
