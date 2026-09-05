package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What a list screen does when the user touches it.
 *
 * Every check here runs without Compose, without a database and without a
 * server, which is the point: a tick on a row is a write against the replica and
 * a sentence back if it was refused, and neither of those is a drawing concern.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TaskListPresenterTest {
    private val sections = MutableStateFlow<List<TaskListSection>>(emptyList())
    private val actions = RecordingActions()
    private val sync = CountingScheduler()

    /** Every write the list can make, remembered rather than performed. */
    private class RecordingActions : TaskListActions {
        val calls = mutableListOf<String>()
        var refuseWith: WriteRefused? = null
        var failWith: RuntimeException? = null

        private fun record(call: String) {
            calls += call
            refuseWith?.let { throw it }
            failWith?.let { throw it }
        }

        override suspend fun complete(taskLocalId: Long) = record("complete $taskLocalId")

        override suspend fun uncomplete(taskLocalId: Long) = record("uncomplete $taskLocalId")

        override suspend fun park(taskLocalId: Long) = record("park $taskLocalId")

        override suspend fun planForWeek(taskLocalId: Long) = record("plan $taskLocalId")
    }

    /** A sync engine that counts requests and can be held open mid-cycle. */
    private class CountingScheduler : SyncScheduler {
        var requests = 0
        var holdUntil: CompletableDeferred<Unit>? = null

        override suspend fun requestSyncNow() {
            requests++
            holdUntil?.await()
        }
    }

    @Test
    fun `the state carries the blocks it was given, and is no longer loading`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            backgroundScope.launch { presenter.state.collect {} }
            sections.value = listOf(plainSection("all", listOf(task(1)), emptyMap()))
            runCurrent()

            assertFalse(presenter.state.value.loading)
            assertEquals(listOf(1L), presenter.state.value.sections.single().rows.map { it.task.localId })
            assertFalse(presenter.state.value.isEmpty)
        }

    @Test
    fun `a list known to hold nothing is empty, an unknown one is not`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            assertFalse(presenter.state.value.isEmpty, "nothing has been read yet, so nothing is known to be empty")

            backgroundScope.launch { presenter.state.collect {} }
            sections.value = listOf(plainSection("all", emptyList(), emptyMap()))
            runCurrent()

            assertTrue(presenter.state.value.isEmpty)
        }

    @Test
    fun `ticking an open task completes it, and ticking a finished one puts it back`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)

            presenter.toggleComplete(task(1))
            presenter.toggleComplete(task(2, status = TaskStatus.COMPLETED))
            runCurrent()

            assertContentEquals(listOf("complete 1", "uncomplete 2"), actions.calls)
        }

    @Test
    fun `a task something still blocks says so instead of queueing a doomed write`() =
        runTest {
            actions.refuseWith = WriteRefused.TaskBlocked(listOf(9))
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            val heard = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }
            runCurrent()

            presenter.toggleComplete(task(1))
            runCurrent()

            assertContentEquals(listOf(TaskListMessage.BLOCKED), heard)
        }

    @Test
    fun `any other refusal, and any failure, says only that the change did not happen`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            val heard = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }
            runCurrent()

            actions.refuseWith = WriteRefused.Placement("a subtask cannot live in the inbox")
            presenter.park(task(1))
            runCurrent()

            actions.refuseWith = null
            actions.failWith = IllegalStateException("the database is gone")
            presenter.planForWeek(task(2))
            runCurrent()

            assertContentEquals(listOf(TaskListMessage.FAILED, TaskListMessage.FAILED), heard)
        }

    @Test
    fun `parking and planning go through the write path untouched`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)

            presenter.park(task(1))
            presenter.planForWeek(task(2))
            runCurrent()

            assertContentEquals(listOf("park 1", "plan 2"), actions.calls)
        }

    @Test
    fun `pulling the list down asks the engine to catch up, and the spinner lasts as long as it does`() =
        runTest {
            val cycle = CompletableDeferred<Unit>()
            sync.holdUntil = cycle
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            backgroundScope.launch { presenter.state.collect {} }
            sections.value = listOf(plainSection("all", emptyList(), emptyMap()))
            runCurrent()

            presenter.refresh()
            runCurrent()
            assertEquals(1, sync.requests)
            assertTrue(presenter.state.value.refreshing, "the gesture is still being answered")

            cycle.complete(Unit)
            runCurrent()
            assertFalse(presenter.state.value.refreshing)
        }

    @Test
    fun `picking tasks starts with the one that was long-pressed and ends when the last is dropped`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            backgroundScope.launch { presenter.state.collect {} }
            sections.value = listOf(plainSection("all", listOf(task(1), task(2)), emptyMap()))
            runCurrent()

            presenter.startSelection(1)
            runCurrent()
            assertTrue(presenter.state.value.selectionMode)
            assertEquals(setOf(1L), presenter.state.value.selected)

            presenter.toggleSelection(2)
            runCurrent()
            assertEquals(setOf(1L, 2L), presenter.state.value.selected)

            presenter.toggleSelection(1)
            presenter.toggleSelection(2)
            runCurrent()
            assertFalse(presenter.state.value.selectionMode, "an empty selection leaves the mode behind")
            assertEquals(emptySet(), presenter.state.value.selected)
        }

    @Test
    fun `leaving selection mode picks nothing`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, actions, sync)
            backgroundScope.launch { presenter.state.collect {} }
            sections.value = emptyList()
            presenter.startSelection(1)
            runCurrent()

            presenter.clearSelection()
            runCurrent()

            assertFalse(presenter.state.value.selectionMode)
            assertEquals(emptySet(), presenter.state.value.selected)
        }
}
