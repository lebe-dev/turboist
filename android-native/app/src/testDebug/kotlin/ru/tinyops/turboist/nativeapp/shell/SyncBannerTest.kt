package ru.tinyops.turboist.nativeapp.shell

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.sync.SyncOutcome
import ru.tinyops.turboist.nativeapp.sync.SyncStatus
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The status strip as it is actually drawn, in each state it has.
 *
 * What the states mean is settled elsewhere, against values. What is left here
 * is the part a person sees: that an app keeping up draws nothing at all, that a
 * running cycle is a hairline rather than a banner, and that the one state where
 * trying again is a real question is the only one that offers a button for it.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SyncBannerTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `an app that is keeping up draws nothing`() {
        show(SyncStatus(outcome = SyncOutcome.SYNCED, lastSyncAt = 1_000))

        compose.onNodeWithText(text(R.string.offline_banner)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.native_sync_failed)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.offline_retry)).assertDoesNotExist()
    }

    @Test
    fun `a running cycle is a hairline and not a banner`() {
        show(SyncStatus(syncing = true, outcome = SyncOutcome.SYNCED, lastSyncAt = 1_000))

        compose.onNodeWithContentDescription(text(R.string.native_sync_inProgress)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.offline_retry)).assertDoesNotExist()
    }

    @Test
    fun `a device that has never synced says only that there is no connection`() {
        show(SyncStatus(outcome = SyncOutcome.UNREACHABLE))

        compose.onNodeWithText(text(R.string.offline_banner)).assertIsDisplayed()
    }

    @Test
    fun `a device that has synced before says how old what is on screen is`() {
        show(SyncStatus(outcome = SyncOutcome.UNREACHABLE, lastSyncAt = 1_700_000_000_000))

        // The moment itself is a wall-clock reading in the device's own zone, so
        // the assertion is on the sentence around it rather than on the clock.
        compose
            .onNodeWithText(text(R.string.offline_bannerStale, "").trim(), substring = true)
            .assertIsDisplayed()
    }

    @Test
    fun `work that has not gone out is counted on the strip`() {
        show(SyncStatus(outcome = SyncOutcome.UNREACHABLE, lastSyncAt = 1_700_000_000_000, waiting = 2))

        compose.onNodeWithText(text(R.string.offline_pendingCount, 2)).assertIsDisplayed()
    }

    @Test
    fun `a turn the server refused offers to try again`() {
        var tries = 0
        show(SyncStatus(outcome = SyncOutcome.REFUSED), onRetry = { tries++ })

        compose.onNodeWithText(text(R.string.native_sync_failed)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.offline_retry)).performClick()

        assertEquals(1, tries)
    }

    @Test
    fun `an unreachable server offers to try again`() {
        var tries = 0
        show(SyncStatus(outcome = SyncOutcome.UNREACHABLE), onRetry = { tries++ })

        compose.onNodeWithText(text(R.string.offline_retry)).performClick()

        assertEquals(1, tries)
    }

    @Test
    fun `a queue that is simply on its way out is stated and not offered a retry`() {
        show(SyncStatus(outcome = SyncOutcome.SYNCED, lastSyncAt = 5, waiting = 3))

        compose.onNodeWithText(text(R.string.offline_pendingCount, 3)).assertIsDisplayed()
        // There is nothing to retry: it goes when the server can be reached, and
        // a button repeating that would be a button that does nothing.
        compose.onNodeWithText(text(R.string.offline_retry)).assertDoesNotExist()
    }

    @Test
    fun `a session nobody could check reads as no connection`() {
        show(SyncStatus(), sessionUnverified = true)

        compose.onNodeWithText(text(R.string.offline_banner)).assertIsDisplayed()
    }

    private fun show(
        status: SyncStatus,
        sessionUnverified: Boolean = false,
        onRetry: () -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                SyncBanner(status = status, sessionUnverified = sessionUnverified, onRetry = onRetry)
            }
        }
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)
}
