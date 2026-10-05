package ru.tinyops.turboist.core.sync.maintenance

import android.util.Log
import kotlinx.coroutines.CancellationException
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult

/**
 * Housekeeping the replica needs from time to time.
 *
 * It is not a schedule of its own, and deliberately so: everything here is about
 * keeping the device's copy the shape the server's is, so the only moment at
 * which it is worth doing — and the only moment at which it can be done from
 * knowledge rather than from a guess — is just after the two have been made to
 * agree.
 */
fun interface ReplicaMaintenance {
    suspend fun runMaintenance()
}

/**
 * The same cycle, with [maintenance] run after each turn that reached the server
 * and agreed with it.
 *
 * Only after such a turn. A turn that could not reach the server left the replica
 * exactly as it was, and doing housekeeping on the strength of it would mean
 * acting on the device's own idea of the time while its copy of the data is known
 * to be behind — throwing away rows on a hunch is the one thing housekeeping must
 * never do.
 *
 * Housekeeping never changes what the turn reports. It is not part of the
 * agreement between the device and the server, and a failure to tidy up must not
 * make a successful catch-up look like a failed one, nor cost the caller its
 * answer — so a step that throws is logged and goes no further, and the turn
 * still reports what it achieved. Cancellation is the exception, and is passed
 * on: it means the scope itself is going away, not that the tidy-up failed.
 */
fun SyncCycle.withMaintenance(maintenance: ReplicaMaintenance): SyncCycle =
    SyncCycle {
        val result = runSyncCycle()
        if (result is SyncCycleResult.Synced) {
            try {
                maintenance.runMaintenance()
            } catch (cancelled: CancellationException) {
                // Not a failed tidy-up: the scope the cycle runs in is being shut
                // down, and swallowing that would keep the coroutine alive past
                // its scope.
                throw cancelled
            } catch (fault: Exception) {
                Log.e(MAINTENANCE_LOG_TAG, "Replica maintenance failed after a successful sync cycle", fault)
            }
        }
        result
    }
