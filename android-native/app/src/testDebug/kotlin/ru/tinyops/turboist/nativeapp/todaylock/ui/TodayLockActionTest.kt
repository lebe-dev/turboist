package ru.tinyops.turboist.nativeapp.todaylock.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import kotlinx.coroutines.flow.MutableSharedFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.todaylock.TodayLockMessage
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The Today lock control, as it is drawn: what it says, that it asks before
 * locking, and that a running lock only explains itself.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TodayLockActionTest {
    @get:Rule
    val compose = createComposeRule()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(
        active: Boolean,
        onLock: () -> Unit = {},
        messages: MutableSharedFlow<TodayLockMessage> = MutableSharedFlow(extraBufferCapacity = 1),
    ) {
        compose.setContent {
            TurboistTheme {
                TodayLockAction(active = active, onLock = onLock, messages = messages)
            }
        }
    }

    @Test
    fun `an open padlock names what it does`() {
        show(active = false)

        compose.onNodeWithContentDescription(text(R.string.page_today_lock_button)).assertIsDisplayed()
    }

    @Test
    fun `a long press explains the lock in a tooltip`() {
        show(active = false)

        compose.onNodeWithTag(TODAY_LOCK_BUTTON_TAG).performTouchInput { longClick() }
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.page_today_lock_tooltip)).assertIsDisplayed()
    }

    @Test
    fun `locking asks first and locks only on confirmation`() {
        var locks = 0
        show(active = false, onLock = { locks++ })

        compose.onNodeWithTag(TODAY_LOCK_BUTTON_TAG).performClick()

        compose.onNodeWithText(text(R.string.page_today_lock_confirmTitle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_today_lock_confirmDescription)).assertIsDisplayed()
        assertEquals(0, locks, "nothing may be locked before the user confirms")

        compose.onNodeWithTag(TODAY_LOCK_CONFIRM_TAG).performClick()

        assertEquals(1, locks)
        compose.onNodeWithText(text(R.string.page_today_lock_confirmTitle)).assertDoesNotExist()
    }

    @Test
    fun `cancelling the confirmation locks nothing`() {
        var locks = 0
        show(active = false, onLock = { locks++ })

        compose.onNodeWithTag(TODAY_LOCK_BUTTON_TAG).performClick()
        compose.onNodeWithText(text(R.string.common_cancel)).performClick()

        assertEquals(0, locks)
        compose.onNodeWithText(text(R.string.page_today_lock_confirmTitle)).assertDoesNotExist()
    }

    @Test
    fun `a running lock only explains itself when tapped`() {
        var locks = 0
        show(active = true, onLock = { locks++ })

        compose.onNodeWithContentDescription(text(R.string.page_today_lock_activeTooltip)).performClick()
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.page_today_lock_confirmTitle)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.page_today_lock_activeTooltip)).assertIsDisplayed()
        assertEquals(0, locks, "a running lock cannot be locked again or lifted")
    }

    @Test
    fun `turning the lock on is announced`() {
        val messages = MutableSharedFlow<TodayLockMessage>(extraBufferCapacity = 1)
        show(active = true, messages = messages)

        compose.runOnIdle { messages.tryEmit(TodayLockMessage.LOCKED) }
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.page_today_lock_locked)).assertIsDisplayed()
    }
}
