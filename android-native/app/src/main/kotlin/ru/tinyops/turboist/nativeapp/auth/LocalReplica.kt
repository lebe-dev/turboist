package ru.tinyops.turboist.nativeapp.auth

import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.sync.drain.OutboxDrainer
import ru.tinyops.turboist.nativeapp.calendar.CalendarCache
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonStore
import ru.tinyops.turboist.nativeapp.labels.RecentLabels
import javax.inject.Inject
import javax.inject.Provider
import javax.inject.Singleton

/**
 * The on-device copy of the user's data, seen from the session layer.
 *
 * Only three things are asked of it here, and all belong to signing out: send
 * what can still be sent, say how much work would be thrown away, and throw it
 * away. Everything else about the replica is the sync engine's business.
 */
interface LocalReplica {
    /**
     * Makes one bounded attempt to send the queued writes, so that a sign-out
     * only ever offers up for loss the work the server really cannot take now.
     *
     * Never throws and never waits long: a device out of coverage must still be
     * able to sign out, and what could not be sent is what is then counted.
     */
    suspend fun sendWaiting()

    /**
     * How many local changes the server has not accepted yet — queued writes and
     * writes it refused alike.
     *
     * Both are lost by a wipe, so both have to be counted before one is offered
     * to the user as a number they are agreeing to discard.
     */
    suspend fun unsentChangeCount(): Int

    /** Empties the replica: the copied records, the queue, and the sync position. */
    suspend fun wipe()
}

/** The [LocalReplica] backed by the real database, plus the stores that sit beside it. */
@Singleton
class RoomLocalReplica
    @Inject
    constructor(
        private val database: TurboistDatabase,
        private val calendar: CalendarCache,
        private val harpoon: HarpoonStore,
        private val recentLabels: RecentLabels,
        // Resolved only when asked: the drainer sits on the HTTP stack, whose
        // token refresh sits on the session layer that holds this replica.
        private val drainer: Provider<OutboxDrainer>,
    ) : LocalReplica {
        override suspend fun sendWaiting() {
            if (database.outbox().count() == 0) return
            val result = withTimeoutOrNull(SEND_TIMEOUT_MILLIS) { drainer.get().drain() }
            if (result == null) {
                Log.i(TAG, "Sending the unsent changes before signing out took too long; counting what is left")
            }
        }

        override suspend fun unsentChangeCount(): Int =
            database.outbox().all().size + database.outbox().quarantined().size

        override suspend fun wipe() {
            // Everything goes, including how far the replica had caught up: a
            // half-emptied copy resumed against a stale cursor would look like a
            // synced replica while missing every record the wipe removed.
            // The call is blocking, so it is kept off whatever thread asked.
            withContext(Dispatchers.IO) { database.clearAllTables() }
            // The kept copy of the user's appointments lives outside the database
            // precisely so a catch-up cannot touch it — which also means clearing
            // the database cannot clear it. Signing out has to say so explicitly,
            // or the next person to use the device is shown the last one's diary.
            calendar.clear()
            // The two things the user was hopping between live outside the
            // database for the same reason, and are the next person's business
            // even less than the diary is.
            harpoon.clear()
            // Recent labels are remembered by the ids this replica gave them, and
            // the next replica hands the same ids out to different labels.
            recentLabels.clear()
        }
    }

private const val TAG = "TurboistSession"

/**
 * How long a sign-out waits for the queue to go out. Long enough for a queue
 * of ordinary size over a slow link, short enough that a user on a dead
 * network is not left looking at a frozen button.
 */
private const val SEND_TIMEOUT_MILLIS = 10_000L
