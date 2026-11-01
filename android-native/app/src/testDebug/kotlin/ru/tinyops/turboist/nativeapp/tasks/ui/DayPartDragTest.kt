package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performTouchInput
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.dayPartSections
import ru.tinyops.turboist.nativeapp.tasks.overdueSection
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * Carrying a row of the day from one phase to another.
 *
 * A long press that moves carries the row; one that does not still starts a
 * selection, which is what a long press did on these lists before rows could
 * be carried at all.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class DayPartDragTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private val taken = mutableListOf<String>()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(
        day: List<Task>,
        overdue: List<Task> = emptyList(),
        drops: Boolean = true,
    ) {
        val state =
            TaskListUiState(
                loading = false,
                sections =
                    listOfNotNull(overdueSection(overdue, emptyMap())) +
                        dayPartSections(day, emptyMap(), DayPart.MORNING),
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
                            onMoveToDayPart =
                                if (drops) {
                                    { task, part -> taken += "phase ${task.localId} $part" }
                                } else {
                                    null
                                },
                        ),
                )
            }
        }
        compose.waitForIdle()
    }

    /** Long-presses [title], then carries it to the root-coordinate point [to] and lets go. */
    private fun carry(
        title: String,
        to: () -> Offset,
    ) {
        // Once the row is picked up the tooltip by the finger names it too, so the
        // row is the first of the two from then on.
        val row = compose.onAllNodesWithText(title).onFirst()
        val rowTopLeft = row.fetchSemanticsNode().boundsInRoot.topLeft
        row.performTouchInput {
            down(center)
            advanceEventTime(viewConfiguration.longPressTimeoutMillis + 100)
            moveBy(Offset(0f, 4f))
        }
        compose.waitForIdle()
        val target = to()
        row.performTouchInput {
            moveTo(target - rowTopLeft)
            up()
        }
        compose.waitForIdle()
    }

    private fun centreOf(title: String): Offset =
        compose.onAllNodesWithText(title).onFirst().fetchSemanticsNode().boundsInRoot.center

    @Test
    fun `a row carried onto another phase's row moves to that phase`() {
        show(
            listOf(
                task(1, title = "Run", dayPart = DayPart.MORNING),
                task(2, title = "Read", dayPart = DayPart.EVENING),
            ),
        )

        carry("Run") { centreOf("Read") }

        assertEquals(listOf("phase 1 EVENING"), taken)
    }

    @Test
    fun `a row carried onto another phase's heading moves to that phase`() {
        show(
            listOf(
                task(1, title = "Run", dayPart = DayPart.MORNING),
                task(2, title = "Read", dayPart = DayPart.EVENING),
            ),
        )
        val evening = text(R.string.task_dayPart_evening)

        // The bar names every phase as well, so the heading is the last of the two.
        carry("Run") { compose.onAllNodesWithText(evening).fetchSemanticsNodes().last().boundsInRoot.center }

        assertEquals(listOf("phase 1 EVENING"), taken)
    }

    @Test
    fun `a phase with nothing in it yet is reached through the bar`() {
        show(listOf(task(1, title = "Run", dayPart = DayPart.MORNING)))
        val afternoon = text(R.string.task_dayPart_afternoon)

        carry("Run") { centreOf(afternoon) }

        assertEquals(listOf("phase 1 AFTERNOON"), taken)
    }

    @Test
    fun `a row let go over its own phase changes nothing`() {
        show(
            listOf(
                task(1, title = "Run", dayPart = DayPart.MORNING),
                task(2, title = "Walk", dayPart = DayPart.MORNING),
            ),
        )

        carry("Run") { centreOf("Walk") }

        assertEquals(emptyList(), taken)
    }

    @Test
    fun `a long press that does not move still starts a selection`() {
        show(listOf(task(1, title = "Run", dayPart = DayPart.MORNING)))

        compose.onNodeWithText("Run").performTouchInput {
            down(center)
            advanceEventTime(viewConfiguration.longPressTimeoutMillis + 100)
            up()
        }
        compose.waitForIdle()

        assertEquals(listOf("select 1"), taken)
    }

    @Test
    fun `an overdue row is not carried into the day`() {
        // Its date has gone by; a new phase would leave it overdue all the same.
        show(
            day = listOf(task(2, title = "Read", dayPart = DayPart.EVENING)),
            overdue = listOf(task(1, title = "Late", dayPart = DayPart.MORNING)),
        )

        carry("Late") { centreOf("Read") }

        assertEquals(emptyList(), taken.filter { it.startsWith("phase") })
    }

    @Test
    fun `a list not cut into phases carries nothing`() {
        show(
            listOf(
                task(1, title = "Run", dayPart = DayPart.MORNING),
                task(2, title = "Read", dayPart = DayPart.EVENING),
            ),
            drops = false,
        )

        carry("Run") { centreOf("Read") }

        assertEquals(emptyList(), taken.filter { it.startsWith("phase") })
    }
}
