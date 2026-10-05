package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.sync.trigger.BackgroundSyncSchedule
import ru.tinyops.turboist.core.sync.trigger.SyncReason
import ru.tinyops.turboist.nativeapp.settings.DeviceOptions
import ru.tinyops.turboist.nativeapp.settings.DeviceOptionsStore
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

/**
 * Whether the background may spend mobile data, and what happens when the answer
 * changes.
 *
 * A job's constraints are fixed at the moment it is handed to the platform, and
 * the schedule keeps an existing repeating job rather than replacing it. So a
 * changed answer that nobody re-applied would take effect only at the next
 * sign-in — which for this setting means exactly the data spend the user just
 * asked the app to stop.
 */
class MeteredSyncPolicyTest {
    private val schedule = RecordingSchedule()
    private val options = FakeDeviceOptionsStore()

    @Test
    fun `the stored answer is readable without suspending, which is how a job is built`() =
        runTest {
            options.value = DeviceOptions(syncOnMetered = false)
            val policy = MeteredSyncPolicy(options, schedule)

            policy.load()

            assertEquals(false, policy.meteredAllowed())
        }

    @Test
    fun `a device that has never been asked lets the background sync`() {
        assertEquals(true, MeteredSyncPolicy(options, schedule).meteredAllowed())
    }

    @Test
    fun `changing the answer rebuilds the repeating job instead of leaving the old constraint in place`() {
        val policy = MeteredSyncPolicy(options, schedule)

        policy.apply(false)

        assertEquals(false, policy.meteredAllowed())
        assertContentEquals(
            listOf("stop", "keep"),
            schedule.calls,
            "the old job has to go before a new one is asked for",
        )
    }

    private class RecordingSchedule : BackgroundSyncSchedule {
        val calls = mutableListOf<String>()

        override fun keepSyncing() {
            calls += "keep"
        }

        override fun syncSoon(reason: SyncReason) {
            calls += "soon"
        }

        override fun stopSyncing() {
            calls += "stop"
        }
    }

    private class FakeDeviceOptionsStore : DeviceOptionsStore {
        var value: DeviceOptions = DeviceOptions()

        override fun observe(): Flow<DeviceOptions> = MutableStateFlow(value)

        override suspend fun read(): DeviceOptions = value

        override suspend fun setTheme(theme: ThemeChoice) {
            value = value.copy(theme = theme)
        }

        override suspend fun setSyncOnMetered(allowed: Boolean) {
            value = value.copy(syncOnMetered = allowed)
        }
    }
}
