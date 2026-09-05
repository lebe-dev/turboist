package ru.tinyops.turboist.core.sync.trigger

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The window that turns a burst of reasons into one round trip.
 *
 * The cost of getting this wrong is not correctness but battery: every extra
 * cycle is a radio wake-up on a device the user is holding, and one change made
 * on another device is several reasons arriving at once.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class SyncSchedulerTest {
    private val cycles = AtomicInteger()

    @Test
    fun `a burst of requests costs one cycle`() =
        runTest {
            val scheduler = scheduler()

            repeat(5) { scheduler.requestSync(SyncReason.SERVER_EVENT) }

            advanceTimeBy(WINDOW - 1)
            runCurrent()
            assertEquals(0, cycles.get(), "nothing runs until the window closes")

            advanceTimeBy(2)
            runCurrent()
            assertEquals(1, cycles.get(), "five reasons to ask the same question cost one round trip")
        }

    @Test
    fun `a request made while a cycle is running opens the next window`() =
        runTest {
            val held = CompletableDeferred<Unit>()
            val scheduler =
                SyncScheduler(
                    cycle = {
                        cycles.incrementAndGet()
                        if (cycles.get() == 1) held.await()
                        SyncCycleResult.Synced
                    },
                    scope = backgroundScope,
                    windowMillis = WINDOW,
                )

            scheduler.requestSync(SyncReason.SERVER_EVENT)
            advanceTimeBy(WINDOW + 1)
            runCurrent()
            assertEquals(1, cycles.get(), "the first cycle has started and is still in flight")

            // Asked while the cycle was on the wire: that cycle read the server
            // before the question existed, so it cannot answer it.
            scheduler.requestSync(SyncReason.SERVER_EVENT)
            held.complete(Unit)
            advanceTimeBy(WINDOW + 1)
            runCurrent()
            assertEquals(2, cycles.get())
        }

    @Test
    fun `asking for a sync now does not wait out the window`() =
        runTest {
            val scheduler = scheduler()

            val result = scheduler.requestSyncNow()

            assertEquals(SyncCycleResult.Synced, result, "the caller is told what became of their request")
            assertEquals(1, cycles.get())
        }

    @Test
    fun `a sync now swallows the request already waiting`() =
        runTest {
            val scheduler = scheduler()

            scheduler.requestSync(SyncReason.SERVER_EVENT)
            backgroundScope.launch { scheduler.requestSyncNow() }
            runCurrent()
            assertEquals(1, cycles.get(), "the immediate run answers the waiting request too")

            advanceTimeBy(WINDOW * 4)
            runCurrent()
            assertEquals(1, cycles.get(), "and no second cycle follows it")
        }

    @Test
    fun `a cycle that fails leaves the next one able to run`() =
        runTest {
            val scheduler =
                SyncScheduler(
                    cycle = {
                        if (cycles.incrementAndGet() == 1) error("the replica could not be written")
                        SyncCycleResult.Synced
                    },
                    scope = backgroundScope,
                    windowMillis = WINDOW,
                )

            scheduler.requestSync(SyncReason.SERVER_EVENT)
            advanceTimeBy(WINDOW + 1)
            runCurrent()
            assertEquals(1, cycles.get())

            scheduler.requestSync(SyncReason.SERVER_EVENT)
            advanceTimeBy(WINDOW + 1)
            runCurrent()

            assertEquals(
                2,
                cycles.get(),
                "the loop that answers every automatic reason runs for the life of the process, " +
                    "so one bad cycle must not be the end of syncing",
            )
        }

    @Test
    fun `a cycle that fails is answered rather than thrown at the caller`() =
        runTest {
            val scheduler =
                SyncScheduler(
                    cycle = { error("the replica could not be written") },
                    scope = backgroundScope,
                    windowMillis = WINDOW,
                )

            val result = scheduler.requestSyncNow()

            assertTrue(
                result is SyncCycleResult.Refused,
                "a screen that pulled to refresh gets an answer; it has no way to handle a crash",
            )
        }

    private fun TestScope.scheduler(): SyncScheduler =
        SyncScheduler(
            cycle = countingCycle(),
            scope = backgroundScope,
            windowMillis = WINDOW,
        )

    private fun countingCycle(): SyncCycle =
        SyncCycle {
            cycles.incrementAndGet()
            SyncCycleResult.Synced
        }

    private companion object {
        const val WINDOW = 500L
    }
}
