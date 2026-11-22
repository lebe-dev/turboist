package ru.tinyops.turboist.nativeapp.switcher.ui

import android.app.Application
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollToIndex
import kotlinx.coroutines.flow.emptyFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.switcher.QuickSwitcherUiState
import ru.tinyops.turboist.nativeapp.switcher.SwitcherRow
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The quick switcher's list composed for real.
 *
 * The panel is opened from every screen's top bar, so the way it answers a
 * refined query is the only thing between the user and what they were looking
 * for: an answer that opens somewhere in the middle reads as the search having
 * gone wrong.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class QuickSwitcherScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val callbacks =
        QuickSwitcherCallbacks(
            onType = {},
            onCreate = {},
            onOpenTask = {},
            onOpenProject = {},
            onDismiss = {},
        )

    private fun row(
        localId: Long,
        title: String,
    ) = SwitcherRow.TaskRow(task(localId, title = title), projectTitle = null)

    private fun answer(
        typed: String,
        rows: List<SwitcherRow>,
    ) = QuickSwitcherUiState(visible = true, typed = typed, rows = rows)

    @Test
    fun `a new answer opens at its first row`() {
        var state by
            mutableStateOf(answer("ma", (1L..20L).map { row(it, "Match $it") }))
        compose.setContent {
            TurboistTheme {
                QuickSwitcherHost(
                    open = true,
                    state = state,
                    messages = emptyFlow(),
                    callbacks = callbacks,
                )
            }
        }

        // The reader is partway down the first answer when a refined query
        // replaces it with one whose rows are new.
        compose.onNodeWithTag(QUICK_SWITCHER_LIST_TAG).performScrollToIndex(15)
        compose.waitForIdle()
        compose.runOnUiThread {
            state = answer("match", (101L..120L).map { row(it, "Fresh $it") })
        }
        compose.waitForIdle()

        compose.onNodeWithText("Fresh 101").assertIsDisplayed()
    }
}
