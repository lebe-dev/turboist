package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.SectionHeading
import ru.tinyops.turboist.nativeapp.tasks.TaskListSection
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.tasks.taskListRows
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId

/**
 * A block that starts folded — today, on the week — shows its heading and
 * nothing under it until the heading is tapped.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TaskListFoldTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    @Test
    fun `a folded block hides its rows until its heading is tapped`() {
        val state =
            TaskListUiState(
                loading = false,
                sections =
                    listOf(
                        TaskListSection(
                            key = "day-today",
                            heading = SectionHeading.Day(today, RelativeDay.TODAY),
                            rows = taskListRows(listOf(task(1, title = "Buy milk")), emptyMap()),
                            foldedByDefault = true,
                        ),
                        TaskListSection(
                            key = "day-tomorrow",
                            heading = SectionHeading.Day(today.plusDays(1), RelativeDay.TOMORROW),
                            rows = taskListRows(listOf(task(2, title = "Call mum")), emptyMap()),
                        ),
                    ),
            )
        compose.setContent {
            TurboistTheme {
                TaskList(
                    state = state,
                    zone = zone,
                    today = today,
                    empty = null,
                    callbacks =
                        TaskListCallbacks(
                            onToggleComplete = {},
                            onOpen = {},
                            onPark = {},
                            onMoveToDay = { _, _ -> },
                            onStartSelection = {},
                            onSelectToggle = {},
                            onClearSelection = {},
                            onRefresh = {},
                        ),
                )
            }
        }

        compose.onNodeWithText("Buy milk").assertDoesNotExist()
        compose.onNodeWithText("Call mum").assertIsDisplayed()

        compose.onNodeWithText(text(R.string.common_today)).performClick()

        compose.onNodeWithText("Buy milk").assertIsDisplayed()
    }
}
