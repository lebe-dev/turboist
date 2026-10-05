package ru.tinyops.turboist.core.sync.trigger

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import ru.tinyops.turboist.core.network.events.ChangeStream
import ru.tinyops.turboist.core.network.events.StreamSignal
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.assertEquals

/**
 * When the app syncs, and — just as important — when it does not.
 *
 * Two of these rules are battery decisions with a user-visible failure mode on
 * either side. A change stream left open in the background drains a phone to
 * learn about tasks nobody is looking at; one that is not open while the app is
 * in front of the user makes another device's edit take a quarter of an hour to
 * appear. Nothing at all happens without a session: there is neither anything to
 * ask nor any right to ask it.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class SyncTriggersTest {
    private val stream = RecordingStream()
    private val networkRegained = MutableSharedFlow<Unit>(extraBufferCapacity = 4)
    private val schedule = RecordingSchedule()
    private val signedIn = MutableStateFlow(true)
    private val foreground = MutableStateFlow(false)
    private val cycles = AtomicInteger()

    @Test
    fun `the change stream is open only while the app is in front of the user`() =
        runTest {
            start()

            assertEquals(0, stream.open.get(), "a backgrounded app holds no stream")

            foreground.value = true
            runCurrent()
            assertEquals(1, stream.open.get())

            foreground.value = false
            runCurrent()
            assertEquals(0, stream.open.get(), "and lets it go the moment the app is put away")
        }

    @Test
    fun `signing out closes the stream and drops the background jobs`() =
        runTest {
            foreground.value = true
            start()
            runCurrent()
            assertEquals(1, stream.open.get())

            signedIn.value = false
            runCurrent()

            assertEquals(0, stream.open.get())
            assertEquals(listOf("keep", "sync:APP_FOREGROUND", "stop"), schedule.calls)
        }

    @Test
    fun `a signed out app never opens a stream, session or not foreground`() =
        runTest {
            signedIn.value = false
            foreground.value = true
            start()
            runCurrent()

            assertEquals(0, stream.open.get())
            assertEquals(listOf("stop"), schedule.calls)
        }

    @Test
    fun `coming to the front asks for a catch-up through the background schedule`() =
        runTest {
            start()

            foreground.value = true
            runCurrent()

            assertEquals(
                listOf("keep", "sync:APP_FOREGROUND"),
                schedule.calls,
                "a job the system accepted survives the app being sent away again; a coroutine would not",
            )
        }

    @Test
    fun `a network that came back asks for a catch-up`() =
        runTest {
            start()

            networkRegained.emit(Unit)
            runCurrent()

            assertEquals(listOf("keep", "sync:NETWORK_REGAINED"), schedule.calls)
        }

    @Test
    fun `a change reported by the server leads to one cycle`() =
        runTest {
            foreground.value = true
            start()
            runCurrent()

            stream.signals.emit(StreamSignal.Changed("tasks"))
            stream.signals.emit(StreamSignal.Changed("plan"))
            advanceTimeBy(WINDOW + 1)
            runCurrent()

            assertEquals(1, cycles.get(), "one edit elsewhere reports several areas and is still one question")
        }

    @Test
    fun `the first opening asks nothing and a reopening asks`() =
        runTest {
            foreground.value = true
            start()
            runCurrent()

            stream.signals.emit(StreamSignal.Opened(afterGap = false))
            advanceTimeBy(WINDOW + 1)
            runCurrent()
            assertEquals(0, cycles.get(), "coming to the front already asked; the stream opening is the same moment")

            stream.signals.emit(StreamSignal.Opened(afterGap = true))
            advanceTimeBy(WINDOW + 1)
            runCurrent()
            assertEquals(1, cycles.get(), "a gap swallowed whatever happened during it, so the gap is the news")
        }

    @Test
    fun `an app with nothing happening asks the server nothing at all`() =
        runTest {
            foreground.value = true
            start()
            runCurrent()
            val onceOpened = schedule.calls.toList()

            // Hours pass with a session, a foreground app and an open stream that
            // reports nothing. Not one cycle may follow: there is no timer in the
            // process, and a device that asked anyway would be spending battery
            // to be told, over and over, that nothing had changed.
            advanceTimeBy(4 * HOUR)
            runCurrent()

            assertEquals(0, cycles.get(), "something in the process is polling the server on a timer")
            assertEquals(
                onceOpened,
                schedule.calls,
                "coming to the front asked once; staying there must not keep asking",
            )
        }

    @Test
    fun `a backgrounded app leaves the waiting to the system`() =
        runTest {
            foreground.value = true
            start()
            runCurrent()

            foreground.value = false
            advanceTimeBy(4 * HOUR)
            runCurrent()

            // Nothing of the app's own runs while it is away: no stream, no
            // cycles. What keeps a backgrounded app from opening on yesterday is
            // the one repeating job handed to the system, which costs this
            // process nothing while it waits.
            assertEquals(0, stream.open.get())
            assertEquals(0, cycles.get())
            assertEquals(1, schedule.calls.count { it == "keep" })
        }

    private fun TestScope.start() {
        val scheduler =
            SyncScheduler(
                cycle =
                    SyncCycle {
                        cycles.incrementAndGet()
                        SyncCycleResult.Synced
                    },
                scope = backgroundScope,
                windowMillis = WINDOW,
            )
        SyncTriggers(
            stream = stream,
            network = { networkRegained },
            schedule = schedule,
            scheduler = scheduler,
            signedIn = signedIn,
            foreground = foreground,
        ).start(backgroundScope)
        runCurrent()
    }

    /** A stream that reports how many collectors it currently has. */
    private class RecordingStream : ChangeStream {
        val open = AtomicInteger()
        val signals = MutableSharedFlow<StreamSignal>(extraBufferCapacity = 8)

        override fun signals(): Flow<StreamSignal> =
            flow {
                open.incrementAndGet()
                try {
                    emitAll(signals)
                } finally {
                    open.decrementAndGet()
                }
            }
    }

    private class RecordingSchedule : BackgroundSyncSchedule {
        val calls = mutableListOf<String>()

        override fun keepSyncing() {
            calls += "keep"
        }

        override fun syncSoon(reason: SyncReason) {
            calls += "sync:${reason.name}"
        }

        override fun stopSyncing() {
            calls += "stop"
        }
    }

    private companion object {
        const val WINDOW = 500L

        /** Long enough that any timer worth worrying about would have fired several times. */
        const val HOUR = 60L * 60L * 1000L
    }
}
