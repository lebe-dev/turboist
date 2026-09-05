package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
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
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * Every state a task row can be in, drawn for real and read back the way a user
 * — or a screen reader — meets it.
 *
 * A row is where the rules of the product become visible: what can be ticked,
 * what is still only on this device, what is urgent, what is entangled with
 * something else. Each of those is asserted through the description a row
 * exposes rather than through its pixels, which is also the thing an assistive
 * reader announces.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TaskRowTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun text(
        resId: Int,
        argument: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, argument)

    /** Midday on [day], which is a due date carrying no time of its own. */
    private fun noon(day: LocalDate): Long = day.atTime(12, 0).atZone(zone).toInstant().toEpochMilli()

    private fun show(
        task: Task,
        projectTitle: String? = null,
        selectionMode: Boolean = false,
        selected: Boolean = false,
        onToggleComplete: () -> Unit = {},
        onOpen: () -> Unit = {},
        onSelectToggle: () -> Unit = {},
        onStartSelection: () -> Unit = {},
    ) {
        setRow {
            TaskRow(
                row = TaskListRow(task = task, depth = 0, projectTitle = projectTitle),
                zone = zone,
                today = today,
                selectionMode = selectionMode,
                selected = selected,
                onToggleComplete = onToggleComplete,
                onOpen = onOpen,
                onSelectToggle = onSelectToggle,
                onStartSelection = onStartSelection,
            )
        }
    }

    private fun setRow(content: @Composable () -> Unit) {
        compose.setContent { TurboistTheme(dynamicColor = false) { content() } }
        compose.waitForIdle()
    }

    @Test
    fun `an open task offers to be ticked off`() {
        var ticked = 0
        show(task(1, title = "Buy milk"), onToggleComplete = { ticked++ })

        compose.onNodeWithText("Buy milk").assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.task_markComplete)).assertIsEnabled().performClick()

        assertEquals(1, ticked)
    }

    @Test
    fun `a task something still blocks shows a padlock and cannot be ticked`() {
        var ticked = 0
        show(
            task(1, relationSummary = TaskRelationSummary(blockedByOpen = 1, total = 1)),
            onToggleComplete = { ticked++ },
        )

        compose.onNodeWithContentDescription(text(R.string.task_blockedTooltip))
            .assertIsDisplayed()
            .assertIsNotEnabled()
            .performClick()

        assertEquals(0, ticked, "a blocked task must not queue a completion the server would refuse")
    }

    @Test
    fun `a finished task offers to be put back`() {
        show(task(1, status = TaskStatus.COMPLETED))

        compose.onNodeWithContentDescription(text(R.string.task_markIncomplete)).assertIsDisplayed()
    }

    @Test
    fun `a finished task that something blocks is still shown as finished`() {
        show(
            task(
                1,
                status = TaskStatus.COMPLETED,
                relationSummary = TaskRelationSummary(blockedByOpen = 1, total = 1),
            ),
        )

        compose.onNodeWithContentDescription(text(R.string.task_markIncomplete)).assertIsDisplayed()
    }

    @Test
    fun `a task the server has never seen says it is waiting to be sent`() {
        show(task(1, serverId = null))

        compose.onNodeWithText(text(R.string.offline_awaitingSend)).assertIsDisplayed()
    }

    @Test
    fun `a task the server knows about says nothing about sending`() {
        show(task(1, serverId = 42))

        compose.onNodeWithText(text(R.string.offline_awaitingSend)).assertDoesNotExist()
    }

    @Test
    fun `the markers a task carries are announced`() {
        show(task(1, isComplex = true, isPrivate = true, isPinned = true))

        compose.onNodeWithContentDescription(text(R.string.task_complexMarker)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.common_privateMarker)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.nav_pinned)).assertIsDisplayed()
    }

    @Test
    fun `a plain task carries no markers`() {
        show(task(1))

        compose.onNodeWithContentDescription(text(R.string.task_complexMarker)).assertDoesNotExist()
        compose.onNodeWithContentDescription(text(R.string.common_privateMarker)).assertDoesNotExist()
        compose.onNodeWithContentDescription(text(R.string.nav_pinned)).assertDoesNotExist()
    }

    @Test
    fun `a due date reads as the day's name where the day has one`() {
        show(task(1, dueAt = noon(today)))

        compose.onNodeWithText(text(R.string.common_today)).assertIsDisplayed()
    }

    @Test
    fun `a habit of putting a task off is worth showing`() {
        show(task(1, postponeCount = 3))

        compose.onNodeWithText(text(R.string.task_postponedTimes, 3)).assertIsDisplayed()
    }

    @Test
    fun `one slip is not worth showing`() {
        show(task(1, postponeCount = 1))

        compose.onNodeWithText(text(R.string.task_postponedTimes, 1)).assertDoesNotExist()
    }

    @Test
    fun `a task entangled with others says how many, and where it lives`() {
        show(
            task(1, relationSummary = TaskRelationSummary(total = 2), recurrenceRule = "FREQ=DAILY"),
            projectTitle = "Kitchen",
        )

        compose.onNodeWithContentDescription(text(R.string.task_relationCountLabel, 2)).assertIsDisplayed()
        compose.onNodeWithText("Kitchen").assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.task_recurringLabel)).assertIsDisplayed()
    }

    @Test
    fun `the labels a task is tagged with are drawn on it`() {
        val tagged =
            task(1).copy(
                labels = listOf(Label(localId = 3, serverId = 3, name = "errand", createdAt = 0, updatedAt = 0)),
            )
        show(tagged)

        compose.onNodeWithText("errand").assertIsDisplayed()
    }

    @Test
    fun `a task committed to the week says so`() {
        show(task(1, planState = PlanState.WEEK))

        compose.onNodeWithContentDescription(text(R.string.task_weekPlannedLabel)).assertIsDisplayed()
    }

    @Test
    fun `a task parked out of the week says so`() {
        show(task(1, planState = PlanState.BACKLOG))

        compose.onNodeWithContentDescription(text(R.string.task_backlogLabel)).assertIsDisplayed()
    }

    @Test
    fun `while tasks are being picked a row offers itself for picking`() {
        var picked = 0
        show(task(1), selectionMode = true, selected = false, onSelectToggle = { picked++ })

        compose.onNodeWithContentDescription(text(R.string.selection_selectTask)).performClick()

        assertEquals(1, picked)
    }

    @Test
    fun `a picked row offers to be dropped again`() {
        show(task(1), selectionMode = true, selected = true)

        compose.onNodeWithContentDescription(text(R.string.selection_unselectTask)).assertIsDisplayed()
    }
}
