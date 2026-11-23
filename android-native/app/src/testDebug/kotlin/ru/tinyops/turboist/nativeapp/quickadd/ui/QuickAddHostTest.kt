package ru.tinyops.turboist.nativeapp.quickadd.ui

import android.app.Application
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddMessage
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The capture surface as it is actually drawn, once something has been written.
 *
 * The presenter is checked on its own elsewhere; what is left here is the part
 * only the screen can answer: that the confirmation a save raises does not take
 * the place of the button the next task starts from. Writing several things in a
 * row is how this surface is used, so the way back in has to survive the message
 * that says the last one landed.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class QuickAddHostTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `the confirmation of a save leaves the button that starts the next task in the clear`() {
        var opened = 0
        show(messages = flowOf(QuickAddMessage.ADDED_TO_INBOX), onOpen = { opened++ })

        compose.waitUntil(TIMEOUT_MILLIS) {
            compose.onAllNodesWithText(text(R.string.task_toast_addedToInbox))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }

        val message = compose.onNodeWithText(text(R.string.task_toast_addedToInbox)).getUnclippedBoundsInRoot()
        val button = compose.onNodeWithContentDescription(text(R.string.task_addTask)).getUnclippedBoundsInRoot()
        assertTrue(message.bottom < button.top, "the message covers the button the next task starts from")

        compose.onNodeWithContentDescription(text(R.string.task_addTask)).performClick()

        assertEquals(1, opened, "the capture button still opens the sheet")
    }

    private fun show(
        messages: Flow<QuickAddMessage> = flowOf(),
        onOpen: () -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                QuickAddHost(
                    state = QuickAddUiState(),
                    today = LocalDate.of(2026, 11, 23),
                    messages = messages,
                    callbacks = QuickAddCallbacks(),
                    onOpen = onOpen,
                )
            }
        }
        compose.waitForIdle()
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private companion object {
        const val TIMEOUT_MILLIS = 5_000L
    }
}
