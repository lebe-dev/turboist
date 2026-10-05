package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.assertEquals
import ru.tinyops.turboist.core.sync.trigger.SyncScheduler as SyncEngine

/**
 * What pulling a list down actually does.
 *
 * A list is a query over the replica, so there is nothing on the screen to
 * refetch: the gesture is the user saying "catch up", and the only honest answer
 * is to run a catch-up and hold the spinner until it is over. Two ways of
 * getting that wrong are worth a test each — dropping the request on the floor,
 * which is what the screens did before the engine existed, and letting a failed
 * catch-up out as an exception into a screen that has nowhere to put it.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ManualRefreshTest {
    private val cycles = AtomicInteger()

    @Test
    fun `pulling a list down runs a catch-up and waits for it`() =
        runTest {
            val refresh = EngineSyncScheduler(engine { SyncCycleResult.Synced })

            refresh.requestSyncNow()

            assertEquals(
                1,
                cycles.get(),
                "the spinner is still up when this returns, so the catch-up has to be over by then",
            )
        }

    @Test
    fun `several screens asking at once each get their catch-up`() =
        runTest {
            val refresh = EngineSyncScheduler(engine { SyncCycleResult.Synced })

            refresh.requestSyncNow()
            refresh.requestSyncNow()

            assertEquals(2, cycles.get(), "a second pull is a second question, asked after the first was answered")
        }

    @Test
    fun `a catch-up that fails does not reach the screen`() =
        runTest {
            val refresh = EngineSyncScheduler(engine { error("the replica could not be written") })

            // Fails the test by throwing: a screen has no way to handle this, and
            // a crash on a pull gesture is the worst possible answer to it.
            refresh.requestSyncNow()

            assertEquals(1, cycles.get())
        }

    private fun TestScope.engine(cycle: SyncCycle): SyncEngine =
        SyncEngine(
            cycle = {
                cycles.incrementAndGet()
                cycle.runSyncCycle()
            },
            scope = backgroundScope,
        )
}
