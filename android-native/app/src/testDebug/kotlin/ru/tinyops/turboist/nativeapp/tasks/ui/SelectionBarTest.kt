package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.dayPartSections
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The bar the list puts up while tasks are being picked.
 *
 * What is checked here is what a finger can reach: that the count is on screen,
 * that each action is described by name for a reader that cannot see the icon,
 * that grouping is offered only once there is something to group, and that a
 * whole block can be picked from its heading.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SelectionBarTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private val taken = mutableListOf<String>()
    private val selectedAll = MutableStateFlow<List<Long>>(emptyList())

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun showSelection(selected: Set<Long>) {
        val rows =
            listOf(
                task(1, title = "Buy milk", dayPart = DayPart.MORNING),
                task(2, title = "Call the vet", dayPart = DayPart.MORNING),
                task(3, title = "Water the plants", dayPart = DayPart.MORNING),
            )
        val state =
            TaskListUiState(
                loading = false,
                sections = dayPartSections(rows, emptyMap(), DayPart.MORNING),
                selectionMode = true,
                selected = selected,
            )
        compose.setContent {
            TurboistTheme(dynamicColor = false) {
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
                            onPlanForWeek = {},
                            onStartSelection = {},
                            onSelectToggle = {},
                            onClearSelection = { taken += "clear" },
                            onRefresh = {},
                            onSelectAllInSection = { selectedAll.value = it },
                            selectionActions =
                                SelectionActions(
                                    onComplete = { taken += "complete" },
                                    onMove = { taken += "move" },
                                    onGroup = { taken += "group" },
                                    onPriority = { taken += "priority ${it.wire}" },
                                    onPlan = { taken += "plan ${it.wire}" },
                                    onDelete = { taken += "delete" },
                                ),
                        ),
                )
            }
        }
        compose.waitForIdle()
    }

    @Test
    fun `the bar counts what is picked and offers a way out`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithText(
            RuntimeEnvironment.getApplication().getString(R.string.selection_bar_count, 2),
        ).assertIsDisplayed()

        compose.onNodeWithContentDescription(text(R.string.selection_bar_cancel)).performClick()
        assertEquals(listOf("clear"), taken)
    }

    @Test
    fun `finishing and filing the selection are on the bar itself`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithContentDescription(text(R.string.selection_bar_complete)).performClick()
        compose.onNodeWithContentDescription(text(R.string.selection_bar_move)).performClick()

        assertEquals(listOf("complete", "move"), taken)
    }

    @Test
    fun `grouping is not offered while one task is picked`() {
        showSelection(setOf(1L))

        val offered =
            compose.onAllNodesWithContentDescription(text(R.string.selection_bar_group))
                .fetchSemanticsNodes()

        assertTrue(offered.isEmpty(), "one task under a new parent is not a group")
    }

    @Test
    fun `grouping is offered from two tasks up`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithContentDescription(text(R.string.selection_bar_group)).performClick()

        assertEquals(listOf("group"), taken)
    }

    @Test
    fun `the rest of the actions are behind the overflow`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithContentDescription(text(R.string.selection_bar_more)).performClick()
        compose.onNodeWithText(text(R.string.task_actions_toBacklog)).performClick()

        assertEquals(listOf("plan ${PlanState.BACKLOG.wire}"), taken)
    }

    @Test
    fun `a priority level is chosen from the overflow`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithContentDescription(text(R.string.selection_bar_more)).performClick()
        compose.onNodeWithText(text(R.string.native_priority_p1)).performClick()

        assertEquals(listOf("priority ${Priority.HIGH.wire}"), taken)
    }

    @Test
    fun `deleting the selection is asked about before it happens`() {
        showSelection(setOf(1L, 2L))

        compose.onNodeWithContentDescription(text(R.string.selection_bar_more)).performClick()
        compose.onNodeWithText(text(R.string.common_delete)).performClick()

        compose.onNodeWithText(text(R.string.selection_confirmDelete_title)).assertIsDisplayed()
        assertTrue(taken.isEmpty(), "nothing is deleted until the question is answered")

        compose.onNodeWithText(text(R.string.common_delete)).performClick()
        assertEquals(listOf("delete"), taken)
    }

    @Test
    fun `a whole block is picked from its heading`() {
        showSelection(setOf(1L))

        compose.onNodeWithText(text(R.string.selection_bar_selectAll)).performClick()

        assertEquals(listOf(1L, 2L, 3L), selectedAll.value)
    }
}
