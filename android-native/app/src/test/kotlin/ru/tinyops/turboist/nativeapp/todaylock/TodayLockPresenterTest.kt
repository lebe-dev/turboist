package ru.tinyops.turboist.nativeapp.todaylock

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Test
import java.time.Instant
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class)
class TodayLockPresenterTest {
    private val now = Instant.parse("2024-03-09T12:00:00Z")
    private val midnight = Instant.parse("2024-03-10T00:00:00Z").toEpochMilli()

    private class Fixture(scope: TestScope, until: Long?, at: Instant, failWith: RuntimeException? = null) {
        val until = MutableStateFlow(until)
        val clock = MutableStateFlow(at)
        var locks = 0
        val presenter =
            TodayLockPresenter(
                scope = scope.backgroundScope,
                lockUntil = this.until,
                now = clock,
                lockToday = {
                    locks++
                    failWith?.let { throw it }
                },
            )
    }

    @Test
    fun `no lock runs before one is turned on`() =
        runTest(UnconfinedTestDispatcher()) {
            val f = Fixture(this, until = null, at = now)
            assertFalse(f.presenter.active.value)
        }

    @Test
    fun `a lock runs until midnight and the clock turns it off`() =
        runTest(UnconfinedTestDispatcher()) {
            val f = Fixture(this, until = midnight, at = now)
            assertTrue(f.presenter.active.value)

            f.clock.value = Instant.ofEpochMilli(midnight)

            assertFalse(f.presenter.active.value, "a screen left open overnight must see the lock run out")
        }

    @Test
    fun `turning it on writes once and says so`() =
        runTest(UnconfinedTestDispatcher()) {
            val f = Fixture(this, until = null, at = now)
            val heard = mutableListOf<TodayLockMessage>()
            backgroundScope.launch { f.presenter.messages.collect { heard += it } }

            f.presenter.lock()
            f.until.value = midnight

            assertEquals(1, f.locks)
            assertEquals(listOf(TodayLockMessage.LOCKED), heard)
            assertTrue(f.presenter.active.value)
        }

    @Test
    fun `a running lock is not locked again`() =
        runTest(UnconfinedTestDispatcher()) {
            val f = Fixture(this, until = midnight, at = now)

            f.presenter.lock()

            assertEquals(0, f.locks)
        }

    @Test
    fun `a failed write is reported and leaves the lock off`() =
        runTest(UnconfinedTestDispatcher()) {
            val f = Fixture(this, until = null, at = now, failWith = IllegalStateException("disk full"))
            val heard = mutableListOf<TodayLockMessage>()
            backgroundScope.launch { f.presenter.messages.collect { heard += it } }

            f.presenter.lock()

            assertEquals(listOf(TodayLockMessage.FAILED), heard)
            assertFalse(f.presenter.active.value)
        }
}
