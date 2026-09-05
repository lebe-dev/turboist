package ru.tinyops.turboist.core.sync.trigger

import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import java.util.concurrent.atomic.AtomicLong

/** Why a sync was asked for. Carried for the log, never for the decision. */
enum class SyncReason {
    /** The server said something changed. */
    SERVER_EVENT,

    /** The change stream came back after a stretch with no stream at all. */
    STREAM_REOPENED,

    /** The device got a network back. */
    NETWORK_REGAINED,

    /** The user brought the app to the front. */
    APP_FOREGROUND,

    /** The background job that keeps a backgrounded app from going stale. */
    PERIODIC,

    /** The user asked, by pulling the list down. */
    MANUAL_REFRESH,
    ;

    /** How the reason reads in a log line. */
    val phrase: String
        get() =
            when (this) {
                SERVER_EVENT -> "the server reported a change"
                STREAM_REOPENED -> "the change stream reopened after a gap"
                NETWORK_REGAINED -> "the network came back"
                APP_FOREGROUND -> "the app came to the front"
                PERIODIC -> "the periodic background job ran"
                MANUAL_REFRESH -> "the user asked for a refresh"
            }
}

/**
 * The one door into the sync engine.
 *
 * Every reason to sync leads here, and what arrives is a *request*, not a run.
 * Requests come in bursts by nature — one edit on another device fans out into
 * several server events, and a device that wakes up gets a network, a
 * foreground and a reopened stream within the same second. Running a cycle for
 * each of them would spend several round trips to learn the same thing once,
 * which on a phone is several radio wake-ups.
 *
 * So a request opens a short window, everything arriving inside it is folded
 * into the same run, and the run happens when the window closes. The window is
 * not extended by later arrivals: a steady stream of events still syncs once per
 * window instead of being starved forever by the next event.
 *
 * A request made while a cycle is running is not folded into it — that cycle
 * read the server before the request existed — so it opens the next window.
 */
class SyncScheduler(
    private val cycle: SyncCycle,
    scope: CoroutineScope,
    private val windowMillis: Long = DEFAULT_WINDOW_MILLIS,
) {
    /** Numbers every request, so a request can be placed in time against a cycle. */
    private val requested = AtomicLong()

    /**
     * The highest request number a finished cycle covers — the count taken as
     * that cycle started, so exactly the requests that predate it.
     */
    private val served = AtomicLong()

    /**
     * Holds at most one waiting request. They all mean the same thing, so a
     * second one has nothing to add beyond its number and the reason in the log.
     */
    private val requests = Channel<Request>(Channel.CONFLATED)

    private data class Request(val reason: SyncReason, val number: Long)

    init {
        scope.launch {
            for (request in requests) {
                delay(windowMillis)
                if (served.get() >= request.number) continue
                runCycle(request.reason)
            }
        }
    }

    /**
     * Asks for a sync, and returns immediately.
     *
     * This is what every automatic trigger uses. Nothing waits for the result:
     * the replica is what the screens read, and it updates when the cycle writes
     * to it.
     */
    fun requestSync(reason: SyncReason) {
        requests.trySend(Request(reason, requested.incrementAndGet()))
    }

    /**
     * Runs a cycle now and answers with what it achieved.
     *
     * For the one trigger that has someone waiting on it: a pull-to-refresh has
     * a spinner attached, and the user pulled precisely because they did not want
     * to wait out a window. A request already waiting in a window is answered by
     * this run and does not turn into a second one — it was made before this
     * cycle went to the server, so this cycle's answer covers it.
     */
    suspend fun requestSyncNow(reason: SyncReason = SyncReason.MANUAL_REFRESH): SyncCycleResult {
        requested.incrementAndGet()
        return runCycle(reason)
    }

    private suspend fun runCycle(reason: SyncReason): SyncCycleResult {
        val covering = requested.get()
        val result = attempt(reason)
        served.updateAndGet { previous -> maxOf(previous, covering) }
        when (result) {
            SyncCycleResult.Synced -> Log.i(LOG_TAG, "Synced because ${reason.phrase}")
            SyncCycleResult.Offline ->
                Log.i(LOG_TAG, "Could not sync because the server was not reachable; asked because ${reason.phrase}")
            // Covers both answers a reached server can give that are not a sync:
            // a refusal it stated, and a failure on this side of the wire. The
            // cause carries which of the two it was.
            is SyncCycleResult.Refused ->
                Log.w(LOG_TAG, "The sync cycle did not complete; asked because ${reason.phrase}", result.cause)
        }
        return result
    }

    /**
     * Runs one cycle and survives one that throws.
     *
     * A cycle answers with a value for everything it expects — a server that
     * cannot be reached, a server that says no — so an exception here is a defect
     * rather than a condition. It still must not escape. The loop above is the
     * only thing that turns an automatic reason into a cycle and it runs for the
     * life of the process: let an exception out and the loop ends, after which
     * the app never syncs by itself again and no screen has anything to say about
     * it. A caller waiting on a cycle is owed the same treatment — a pull gesture
     * has nowhere to put a crash.
     */
    private suspend fun attempt(reason: SyncReason): SyncCycleResult =
        try {
            cycle.runSyncCycle()
        } catch (cancelled: CancellationException) {
            // Not a failed cycle: the scope this runs in is being shut down, and
            // swallowing that would keep the coroutine alive past its scope.
            throw cancelled
        } catch (fault: Exception) {
            Log.e(LOG_TAG, "The sync cycle failed unexpectedly; asked because ${reason.phrase}", fault)
            SyncCycleResult.Refused(fault)
        }

    companion object {
        internal const val LOG_TAG = "TurboistSync"

        /**
         * How long a request waits for company before it turns into a cycle.
         *
         * Long enough to swallow the fan-out of a single change on another device,
         * short enough that the wait is not what the user notices.
         */
        const val DEFAULT_WINDOW_MILLIS: Long = 500
    }
}
