package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.dayPartSections
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * The swipe that uncovers a row's actions.
 *
 * Swiping a row towards the start used to move it to the backlog the moment the
 * finger let go, which is an easy gesture to make by accident while scrolling.
 * It now only uncovers the actions behind the row, and nothing happens until one
 * of them is tapped — which is what is checked here.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TaskRowSwipeTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private val taken = mutableListOf<String>()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(row: Task) {
        val state =
            TaskListUiState(
                loading = false,
                sections = dayPartSections(listOf(row), emptyMap(), DayPart.MORNING),
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
                            onToggleComplete = { taken += "complete ${it.localId}" },
                            onOpen = { taken += "open ${it.localId}" },
                            onPark = { taken += "park ${it.localId}" },
                            onMoveToDay = { task, day -> taken += "day ${task.localId} $day" },
                            onStartSelection = { taken += "select ${it.localId}" },
                            onSelectToggle = {},
                            onClearSelection = {},
                            onRefresh = {},
                        ),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun swipeTowardsStart(title: String) {
        compose.onNodeWithText(title).performTouchInput { swipeLeft() }
        compose.waitForIdle()
    }

    @Test
    fun `the actions are hidden until the row is swiped`() {
        show(task(1, title = "Buy milk", dayPart = DayPart.MORNING))

        compose.onNodeWithText(text(R.string.native_swipe_backlog)).assertDoesNotExist()
    }

    @Test
    fun `swiping towards the start uncovers the actions and takes none of them`() {
        show(task(1, title = "Buy milk", dayPart = DayPart.MORNING))

        swipeTowardsStart("Buy milk")

        compose.onNodeWithText(text(R.string.native_swipe_backlog)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.common_tomorrow)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_swipe_select)).assertIsDisplayed()
        assertEquals(emptyList(), taken)
    }

    @Test
    fun `an uncovered action is taken only when it is tapped`() {
        show(task(1, title = "Buy milk", dayPart = DayPart.MORNING))

        swipeTowardsStart("Buy milk")
        compose.onNodeWithText(text(R.string.native_swipe_backlog)).performClick()

        assertEquals(listOf("park 1"), taken)
    }

    @Test
    fun `a parked task is offered tomorrow and not the backlog it is already in`() {
        show(task(1, title = "Buy milk", dayPart = DayPart.MORNING, planState = PlanState.BACKLOG))

        swipeTowardsStart("Buy milk")
        compose.onNodeWithText(text(R.string.native_swipe_backlog)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.common_tomorrow)).performClick()

        assertEquals(listOf("day 1 ${today.plusDays(1)}"), taken)
    }

    @Test
    fun `a task already on tomorrow is not offered tomorrow again`() {
        val tomorrow = today.plusDays(1).atStartOfDay(zone).toInstant().toEpochMilli()
        show(task(1, title = "Buy milk", dayPart = DayPart.MORNING, dueAt = tomorrow))

        swipeTowardsStart("Buy milk")

        // The row's own due label still says when the task is; only the action is gone.
        compose.onNodeWithText(text(R.string.native_swipe_backlog)).assertIsDisplayed()
        compose.onAllNodesWithText(text(R.string.common_tomorrow)).assertCountEquals(1)
    }
}
