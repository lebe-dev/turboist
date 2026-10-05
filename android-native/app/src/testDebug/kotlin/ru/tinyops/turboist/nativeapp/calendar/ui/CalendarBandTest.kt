package ru.tinyops.turboist.nativeapp.calendar.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.SectionHeading
import ru.tinyops.turboist.nativeapp.tasks.TaskListSection
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.tasks.taskListRows
import ru.tinyops.turboist.nativeapp.tasks.ui.EmptyListText
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskList
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskListCallbacks
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * A day drawn with appointments on it.
 *
 * Two claims are checked here rather than in the plain grouping tests, because
 * both are about what reaches the screen: an appointment is visible beside the
 * work of the same phase, and it is *not* a task — none of the things a row
 * offers appear on it. The third is the offline line, which is the whole of what
 * this client says about a calendar it could not reach.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class CalendarBandTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun at(hour: Int): Long = ZonedDateTime.of(today, LocalTime.of(hour, 0), zone).toInstant().toEpochMilli()

    private val callbacks =
        TaskListCallbacks(
            onToggleComplete = {},
            onOpen = {},
            onPark = {},
            onPlanForWeek = {},
            onStartSelection = {},
            onSelectToggle = {},
            onClearSelection = {},
            onRefresh = {},
        )

    private fun show(state: TaskListUiState) {
        compose.setContent {
            TurboistTheme {
                TaskList(state = state, zone = zone, today = today, empty = null, callbacks = callbacks)
            }
        }
        compose.waitForIdle()
    }

    private fun morning(
        tasks: List<Long>,
        events: List<CalendarEvent>,
    ) = TaskListSection(
        key = "phase-MORNING",
        heading = SectionHeading.Phase(DayPart.MORNING, active = true),
        rows = taskListRows(tasks.map { task(it, title = "task $it") }, emptyMap()),
        events = events,
    )

    @Test
    fun `an appointment is drawn beside the work of the same part of the day`() {
        val standup = CalendarEvent(id = "e1", title = "Standup", startsAt = at(9), endsAt = at(10))
        show(TaskListUiState(loading = false, sections = listOf(morning(listOf(1L), listOf(standup)))))

        compose.onNodeWithText("task 1").assertIsDisplayed()
        compose.onNodeWithText("Standup").assertIsDisplayed()
    }

    @Test
    fun `an appointment offers none of the things a task row offers`() {
        val standup = CalendarEvent(id = "e1", title = "Standup", startsAt = at(9), endsAt = at(10))
        show(TaskListUiState(loading = false, sections = listOf(morning(emptyList(), listOf(standup)))))

        compose.onNodeWithText("Standup").assertIsDisplayed()
        compose
            .onNodeWithContentDescription(text(R.string.task_markComplete))
            .assertDoesNotExist()
    }

    @Test
    fun `a whole-day appointment says so instead of showing times`() {
        val away = CalendarEvent(id = "e2", title = "Away", allDay = true, startDay = today)
        show(TaskListUiState(loading = false, sections = listOf(morning(emptyList(), listOf(away)))))

        compose.onNodeWithText(text(R.string.calendar_allDay), substring = true).assertIsDisplayed()
    }

    @Test
    fun `a day holding only appointments is not an empty day`() {
        val standup = CalendarEvent(id = "e1", title = "Standup", startsAt = at(9), endsAt = at(10))
        val state = TaskListUiState(loading = false, sections = listOf(morning(emptyList(), listOf(standup))))

        compose.setContent {
            TurboistTheme {
                TaskList(
                    state = state,
                    zone = zone,
                    today = today,
                    empty =
                        EmptyListText(
                            title = text(R.string.page_today_emptyTitle),
                            description = text(R.string.page_today_emptyDescription),
                        ),
                    callbacks = callbacks,
                )
            }
        }
        compose.waitForIdle()

        compose.onNodeWithText("Standup").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_today_emptyTitle)).assertDoesNotExist()
    }

    @Test
    fun `entries that could not be refreshed are shown with the moment they were read`() {
        val standup = CalendarEvent(id = "e1", title = "Standup", startsAt = at(9), endsAt = at(10))
        show(
            TaskListUiState(
                loading = false,
                sections = listOf(morning(emptyList(), listOf(standup))),
                calendarAsOf = at(8),
            ),
        )

        compose.onNodeWithText("Standup").assertIsDisplayed()
        compose.onNodeWithText("Calendar as of", substring = true).assertIsDisplayed()
    }

    @Test
    fun `nothing is said about the calendar while it is current`() {
        val standup = CalendarEvent(id = "e1", title = "Standup", startsAt = at(9), endsAt = at(10))
        show(TaskListUiState(loading = false, sections = listOf(morning(emptyList(), listOf(standup)))))

        compose.onNodeWithText("Calendar as of", substring = true).assertDoesNotExist()
    }
}
