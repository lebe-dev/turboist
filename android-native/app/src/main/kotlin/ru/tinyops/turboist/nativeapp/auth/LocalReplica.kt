package ru.tinyops.turboist.nativeapp.auth

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.nativeapp.calendar.CalendarCache
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonStore
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The on-device copy of the user's data, seen from the session layer.
 *
 * Only two questions are asked of it here, and both belong to signing out: how
 * much work would be thrown away, and throw it away. Everything else about the
 * replica is the sync engine's business.
 */
interface LocalReplica {
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
    ) : LocalReplica {
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
        }
    }
