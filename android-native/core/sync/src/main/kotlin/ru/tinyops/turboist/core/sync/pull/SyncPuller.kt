package ru.tinyops.turboist.core.sync.pull

import android.util.Log
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.api.SyncApi
import ru.tinyops.turboist.core.sync.SyncMutex
import java.io.IOException
import java.util.concurrent.atomic.AtomicLong

/**
 * Brings the replica level with the server.
 *
 * There are only two ways to do that and this picks between them: a device that
 * has never synced takes a complete copy, and one that has takes the changes
 * since the position it stopped at, a page at a time. Everything else here is
 * about the ways that can go wrong.
 *
 * The server can refuse to continue a catch-up for two reasons, and both mean
 * the same thing to this class — the position is meaningless now, so start from
 * a complete copy. Either the history was replaced, which is what restoring a
 * backup does, or the part still needed has been pruned because the device was
 * away for longer than the server keeps.
 *
 * Losing the network is not one of those reasons. It is reported as a value and
 * nothing is written; every screen goes on reading the replica it already has,
 * and the caller decides when to try again.
 */
class SyncPuller(
    private val api: SyncApi,
    private val db: TurboistDatabase,
    private val applier: ReplicaApplier,
    private val mutex: SyncMutex,
    private val pageSize: Int = DEFAULT_PAGE_SIZE,
) {
    /**
     * Numbers every request, so a request can be placed in time against the
     * catch-up that happens to be running when it arrives.
     */
    private val requests = AtomicLong()

    /**
     * The highest request number the last completed catch-up covers — the count
     * taken as it started, so exactly the requests that predate it. Both fields
     * are read and written under the lock.
     */
    private var servedUpTo: Long = 0
    private var lastResult: PullResult? = null

    /**
     * Catches the replica up, or reports why it could not be.
     *
     * Callers ask for this from everywhere — a screen refreshing, a background
     * job waking up, a server event arriving — and several of them regularly ask
     * at once. A catch-up may answer a caller only if it *began after that
     * caller asked*: it went looking for changes with the question already
     * standing, so its answer covers it. Every caller queued up when a catch-up
     * starts is therefore served by that one, and a burst costs one round trip.
     *
     * A caller that asks once a catch-up is already on the wire is a younger
     * question, and gets a round trip of its own — the running catch-up read the
     * server before that caller existed, so a change made in between would go
     * unnoticed until something else happened to ask.
     *
     * The one lock the sync cycle has is held throughout, so this never runs
     * beside the sending half — a read that overlapped a send could write the
     * server's pre-send state over the very rows the user just changed.
     */
    suspend fun pull(): PullResult {
        val request = requests.incrementAndGet()
        return mutex.withExclusiveAccess {
            val alreadyAnswered = lastResult
            if (alreadyAnswered != null && servedUpTo >= request) {
                return@withExclusiveAccess alreadyAnswered
            }
            val serving = requests.get()
            val result = catchUp()
            servedUpTo = serving
            lastResult = result
            result
        }
    }

    private suspend fun catchUp(): PullResult {
        val state = db.syncState().get() ?: return takeCompleteCopy()
        var epoch = state.epoch
        var cursor = state.cursor
        var records = 0
        while (true) {
            val page =
                try {
                    api.changes(since = cursor, epoch = epoch, limit = pageSize)
                } catch (replaced: ApiException.SyncEpochMismatch) {
                    Log.i(
                        PULL_LOG_TAG,
                        "The server's change history was replaced; taking a complete copy instead of catching up",
                        replaced,
                    )
                    return takeCompleteCopy()
                } catch (pruned: ApiException.SyncCursorExpired) {
                    Log.i(
                        PULL_LOG_TAG,
                        "This device was away longer than the server keeps its change history; taking a complete copy",
                        pruned,
                    )
                    return takeCompleteCopy()
                } catch (unreachable: ApiException.Network) {
                    return PullResult.Offline(unreachable)
                } catch (refused: ApiException) {
                    return PullResult.Refused(refused)
                } catch (transport: IOException) {
                    return PullResult.Offline(
                        ApiException.Network(transport.message ?: "the request did not reach the server", transport),
                    )
                }

            when (val outcome = applier.applyPage(page)) {
                is PageOutcome.Applied -> {
                    records += outcome.records
                    val moved = outcome.cursor > cursor
                    epoch = page.epoch
                    cursor = outcome.cursor
                    if (!page.hasMore) return PullResult.Applied(epoch, cursor, records, fromSnapshot = false)
                    if (!moved) {
                        // The server says there is more but did not move the
                        // position, so asking again would fetch the same page
                        // forever. Stop here; the next request starts clean.
                        Log.w(PULL_LOG_TAG, "The server offered more changes without moving the position; stopping")
                        return PullResult.Applied(epoch, cursor, records, fromSnapshot = false)
                    }
                }

                PageOutcome.Unresolvable -> return takeCompleteCopy()
            }
        }
    }

    private suspend fun takeCompleteCopy(): PullResult {
        val snapshot =
            try {
                api.snapshot()
            } catch (unreachable: ApiException.Network) {
                return PullResult.Offline(unreachable)
            } catch (refused: ApiException) {
                return PullResult.Refused(refused)
            } catch (transport: IOException) {
                return PullResult.Offline(
                    ApiException.Network(transport.message ?: "the request did not reach the server", transport),
                )
            }
        val records = applier.applySnapshot(snapshot)
        Log.i(PULL_LOG_TAG, "Seeded the replica with $records records at position ${snapshot.cursor}")
        return PullResult.Applied(snapshot.epoch, snapshot.cursor, records, fromSnapshot = true)
    }

    companion object {
        /**
         * How many changes to ask for at a time. It is the largest page the server
         * will serve: the catch-up is one round trip per page over a mobile link,
         * and each round trip costs far more than the rows it carries.
         */
        const val DEFAULT_PAGE_SIZE: Int = 500
    }
}
