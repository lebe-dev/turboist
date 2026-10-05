package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import ru.tinyops.turboist.nativeapp.tasks.TaskListPresenter
import ru.tinyops.turboist.nativeapp.tasks.dayPartSections
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * A day screen driven end to end, with a stand-in for the replica behind it.
 *
 * The fake below is a list of tasks and the four writes, and it behaves the way
 * the real write path does: a change is applied where the screen reads from,
 * inside the call, with nothing asked of a server. That is the whole claim being
 * checked here — a task ticked off leaves the day it was on immediately, whether
 * or not the device has a network — so the sync engine is a counter that must
 * stay at zero.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TaskListScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private val tasks = MutableStateFlow<List<Task>>(emptyList())
    private val sync = CountingScheduler()
    private var refuseWith: WriteRefused? = null
    private lateinit var presenter: TaskListPresenter

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    /** A sync engine nobody is supposed to need for any of this. */
    private class CountingScheduler : SyncScheduler {
        var requests = 0

        override suspend fun requestSyncNow() {
            requests++
        }
    }

    /**
     * The replica, as far as a screen can tell: the writes land in it, and the
     * list the screen is reading changes because of that and nothing else.
     */
    private inner class FakeReplica : TaskListActions {
        override suspend fun complete(taskLocalId: Long) {
            refuseWith?.let { throw it }
            // A completed task leaves the day view, exactly as the query behind
            // the real screen stops matching it.
            tasks.value = tasks.value.filterNot { it.localId == taskLocalId }
        }

        override suspend fun uncomplete(taskLocalId: Long) {
            refuseWith?.let { throw it }
            tasks.value = tasks.value.map { if (it.localId == taskLocalId) it.copy(status = TaskStatus.OPEN) else it }
        }

        override suspend fun park(taskLocalId: Long) {
            refuseWith?.let { throw it }
        }

        override suspend fun planForWeek(taskLocalId: Long) {
            refuseWith?.let { throw it }
        }
    }

    private fun showDay(empty: EmptyListText? = dayEmptyText()) {
        val sections = tasks.map { dayPartSections(it, emptyMap(), DayPart.MORNING) }
        compose.setContent {
            val scope = rememberCoroutineScope()
            val screen =
                remember {
                    TaskListPresenter(scope, sections, FakeReplica(), sync).also { presenter = it }
                }
            val state by screen.state.collectAsState()
            TurboistTheme {
                TaskListScreen(
                    state = state,
                    zone = zone,
                    today = today,
                    empty = empty,
                    messages = screen.messages,
                    callbacks =
                        TaskListCallbacks(
                            onToggleComplete = screen::toggleComplete,
                            onOpen = {},
                            onPark = screen::park,
                            onPlanForWeek = screen::planForWeek,
                            onStartSelection = { screen.startSelection(it.localId) },
                            onSelectToggle = { screen.toggleSelection(it.localId) },
                            onClearSelection = screen::clearSelection,
                            onRefresh = screen::refresh,
                        ),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun dayEmptyText() =
        EmptyListText(
            title = text(R.string.page_today_emptyTitle),
            description = text(R.string.page_today_emptyDescription),
        )

    @Test
    fun `the day is drawn from what the device already holds`() {
        tasks.value = listOf(task(1, title = "Buy milk", dayPart = DayPart.MORNING), task(2, title = "Call the vet"))
        showDay()

        compose.onNodeWithText("Buy milk").assertIsDisplayed()
        compose.onNodeWithText("Call the vet").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.task_dayPart_morning)).assertIsDisplayed()
        assertEquals(0, sync.requests, "a list is a query, so drawing one asks the server for nothing")
    }

    @Test
    fun `ticking a task off takes it out of the day at once, with no network`() {
        tasks.value = listOf(task(1, title = "Buy milk"), task(2, title = "Call the vet"))
        showDay()

        compose.onNodeWithText("Buy milk").assertIsDisplayed()
        compose.onAllNodesWithContentDescription(text(R.string.task_markComplete))[0].performClick()
        compose.waitForIdle()

        compose.onNodeWithText("Buy milk").assertDoesNotExist()
        compose.onNodeWithText("Call the vet").assertIsDisplayed()
        assertEquals(0, sync.requests)
    }

    @Test
    fun `a refused completion is said in the user's own words`() {
        refuseWith = WriteRefused.TaskBlocked(listOf(9))
        tasks.value = listOf(task(1, title = "Buy milk"))
        showDay()

        compose.onNodeWithContentDescription(text(R.string.task_markComplete)).performClick()
        compose.waitUntil(TIMEOUT_MILLIS) {
            compose.onAllNodesWithText(text(R.string.task_toast_blockedCannotComplete))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }

        compose.onNodeWithText("Buy milk").assertIsDisplayed()
    }

    @Test
    fun `a day with nothing on it says so`() {
        tasks.value = emptyList()
        showDay()

        compose.onNodeWithText(text(R.string.page_today_emptyTitle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_today_emptyDescription)).assertIsDisplayed()
    }

    @Test
    fun `a list with no wording for its own emptiness draws no placeholder`() {
        tasks.value = emptyList()
        showDay(empty = null)

        compose.onNodeWithText(text(R.string.page_today_emptyTitle)).assertDoesNotExist()
    }

    @Test
    fun `pulling the list down asks the sync engine to catch up`() {
        tasks.value = listOf(task(1, title = "Buy milk"))
        showDay()

        compose.runOnIdle { presenter.refresh() }
        compose.waitForIdle()

        assertEquals(1, sync.requests)
    }

    private companion object {
        /** Long enough for a snackbar to be raised, short enough to fail a test that hangs. */
        const val TIMEOUT_MILLIS = 5_000L
    }
}
