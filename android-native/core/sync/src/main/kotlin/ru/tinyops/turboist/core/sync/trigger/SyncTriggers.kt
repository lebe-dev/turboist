package ru.tinyops.turboist.core.sync.trigger

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.network.events.ChangeStream
import ru.tinyops.turboist.core.network.events.StreamSignal

/**
 * Everything that decides when to sync, in one place.
 *
 * There are five reasons and they are not interchangeable, because they differ
 * in what is true when they happen:
 *
 * - **The server said something changed.** The freshest reason there is, and the
 *   only one that costs nothing to wait for — but only while the app is in front
 *   of the user. Holding a connection open in the background drains a battery for
 *   nothing, and the system suspends it anyway.
 * - **The stream reopened after a gap.** Changes made during the gap arrived as
 *   silence, so the gap itself is the news.
 * - **The network came back.** Whatever was queued while it was gone can go now.
 * - **The app came to the front.** The user is about to read the data, which is
 *   the moment its age starts to matter.
 * - **The clock.** What keeps a backgrounded app from opening on yesterday.
 *
 * The first two are this object's own work; the last three it hands to the
 * background schedule, so they still happen when the process does not exist.
 *
 * Nothing runs while signed out. A device with no session has nothing to ask the
 * server and no right to ask it, and a background job that wakes a radio to be
 * told so is worse than no job at all.
 */
class SyncTriggers(
    private val stream: ChangeStream,
    private val network: NetworkAvailability,
    private val schedule: BackgroundSyncSchedule,
    private val scheduler: SyncScheduler,
    private val signedIn: Flow<Boolean>,
    private val foreground: Flow<Boolean>,
) {
    /**
     * Starts reacting, for as long as [scope] lives.
     *
     * The scope belongs to the process rather than to a screen: a sync that a
     * screen started must not be cancelled because the user navigated away
     * halfway through it.
     */
    fun start(scope: CoroutineScope): Job =
        scope.launch {
            signedIn.distinctUntilChanged().collectLatest { hasSession ->
                if (!hasSession) {
                    schedule.stopSyncing()
                    return@collectLatest
                }
                schedule.keepSyncing()
                coroutineScope {
                    launch { network.regained().collect { schedule.syncSoon(SyncReason.NETWORK_REGAINED) } }
                    launch { followForeground() }
                }
            }
        }

    /**
     * Keeps a change stream open exactly while the app is in front of the user,
     * and asks for a catch-up each time it gets there.
     *
     * The catch-up goes through the background schedule rather than straight to
     * the engine: the app can be sent away again immediately, and a job the
     * system has accepted survives that, while a coroutine started for the
     * occasion does not.
     */
    private suspend fun followForeground() {
        foreground.distinctUntilChanged().collectLatest { inFront ->
            if (!inFront) return@collectLatest
            schedule.syncSoon(SyncReason.APP_FOREGROUND)
            stream.signals().collect(::react)
        }
    }

    private fun react(signal: StreamSignal) {
        when (signal) {
            // The first opening of a session is not a reason to ask: whoever
            // brought the app to the front has already asked. An opening after a
            // gap is, because the gap swallowed whatever happened during it.
            is StreamSignal.Opened -> if (signal.afterGap) scheduler.requestSync(SyncReason.STREAM_REOPENED)
            // Which area changed is not read. Every event is answered the same
            // way — by asking for everything that changed since the stored
            // position — so the answer covers areas the event never mentioned.
            is StreamSignal.Changed -> scheduler.requestSync(SyncReason.SERVER_EVENT)
            is StreamSignal.Closed -> Log.d(SyncScheduler.LOG_TAG, "The change stream is down")
        }
    }
}
