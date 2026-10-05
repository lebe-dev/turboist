package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What a list does with a picked set of tasks.
 *
 * The claims here are the ones a user would notice going wrong: an action reaches
 * the write path once with the whole selection rather than once per task, the
 * mode is left behind afterwards, and a completion that could not finish
 * everything says so instead of quietly reporting a smaller number.
 *
 * None of it needs Compose or a database — the port below records what it was
 * asked for, which is exactly what these cases are about.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class BulkSelectionTest {
    private val sections = MutableStateFlow<List<TaskListSection>>(emptyList())
    private val rowActions = NoRowActions()
    private val bulk = RecordingBulkActions()
    private val sync = SilentScheduler()

    /** Every call the selection made, and what it was told to do with it. */
    private class RecordingBulkActions : BulkTaskActions {
        val calls = mutableListOf<Pair<String, List<Long>>>()
        var blocked: Int = 0
        var failWith: WriteRefused? = null

        private fun record(
            name: String,
            ids: List<Long>,
        ): BulkChange {
            calls += name to ids
            failWith?.let { throw it }
            return BulkChange(changed = ids.size - blocked, leftBlocked = blocked)
        }

        override suspend fun complete(taskLocalIds: List<Long>) = record("complete", taskLocalIds)

        override suspend fun move(
            taskLocalIds: List<Long>,
            destination: TaskDestination,
        ) = record("move to $destination", taskLocalIds)

        override suspend fun prioritise(
            taskLocalIds: List<Long>,
            priority: Priority,
        ) = record("prioritise ${priority.wire}", taskLocalIds)

        override suspend fun plan(
            taskLocalIds: List<Long>,
            state: PlanState,
        ) = record("plan ${state.wire}", taskLocalIds)

        override suspend fun delete(taskLocalIds: List<Long>) = record("delete", taskLocalIds)

        override suspend fun group(
            title: String,
            childTaskLocalIds: List<Long>,
            destination: TaskDestination,
        ) = record("group \"$title\" into $destination", childTaskLocalIds)
    }

    /** The row writes, which none of these cases makes. */
    private class NoRowActions : TaskListActions {
        override suspend fun complete(taskLocalId: Long) = Unit

        override suspend fun uncomplete(taskLocalId: Long) = Unit

        override suspend fun park(taskLocalId: Long) = Unit

        override suspend fun planForWeek(taskLocalId: Long) = Unit
    }

    private class SilentScheduler : SyncScheduler {
        override suspend fun requestSyncNow() = Unit
    }

    @Test
    fun `each action reaches the write path once, carrying the whole selection`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            presenter.startSelection(1)
            presenter.toggleSelection(2)
            presenter.toggleSelection(3)

            presenter.completeSelected()
            runCurrent()

            assertEquals(1, bulk.calls.size, "a selection action is one call, not one call per task")
            assertEquals("complete", bulk.calls.single().first)
            assertContentEquals(listOf(1L, 2L, 3L), bulk.calls.single().second)
        }

    @Test
    fun `the other five actions carry the selection too, each named by what it does`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)

            for (action in listOf<(TaskListPresenter) -> Unit>(
                { it.moveSelected(TaskDestination.InProject(9)) },
                { it.prioritiseSelected(Priority.HIGH) },
                { it.planSelected(PlanState.WEEK) },
                { it.planSelected(PlanState.BACKLOG) },
                { it.deleteSelected() },
                { it.groupSelected("Launch", TaskDestination.InProject(9)) },
            )) {
                presenter.startSelection(1)
                presenter.toggleSelection(2)
                action(presenter)
                runCurrent()
            }

            assertContentEquals(
                listOf(
                    "move to " + TaskDestination.InProject(9),
                    "prioritise high",
                    "plan week",
                    "plan backlog",
                    "delete",
                    "group \"Launch\" into " + TaskDestination.InProject(9),
                ),
                bulk.calls.map { it.first },
            )
            assertTrue(bulk.calls.all { it.second == listOf(1L, 2L) })
        }

    @Test
    fun `a completion that could not finish everything says how much is still in the way`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            val heard = mutableListOf<BulkOutcome>()
            backgroundScope.launch { presenter.bulkOutcomes.collect { heard += it } }
            bulk.blocked = 2
            presenter.startSelection(1)
            presenter.toggleSelection(2)
            presenter.toggleSelection(3)

            presenter.completeSelected()
            runCurrent()

            assertContentEquals(listOf(BulkOutcome(BulkAction.COMPLETED, changed = 1, leftBlocked = 2)), heard)
        }

    @Test
    fun `an action leaves selection mode behind it`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            backgroundScope.launch { presenter.state.collect {} }
            presenter.startSelection(1)
            runCurrent()
            assertTrue(presenter.state.value.selectionMode)

            presenter.deleteSelected()
            runCurrent()

            assertFalse(presenter.state.value.selectionMode, "the tasks it acted on have left the list")
            assertTrue(presenter.state.value.selected.isEmpty())
        }

    @Test
    fun `an action that fails reports it and keeps the selection to try again`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            val said = mutableListOf<TaskListMessage>()
            val outcomes = mutableListOf<BulkOutcome>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            backgroundScope.launch { presenter.bulkOutcomes.collect { outcomes += it } }
            backgroundScope.launch { presenter.state.collect {} }
            bulk.failWith = WriteRefused.Placement("nowhere to put it")
            presenter.startSelection(1)
            runCurrent()

            presenter.moveSelected(TaskDestination.Inbox)
            runCurrent()

            assertContentEquals(listOf(TaskListMessage.FAILED), said)
            assertTrue(outcomes.isEmpty(), "nothing changed, so there is nothing to report as done")
            assertTrue(presenter.state.value.selectionMode, "the picked tasks are still picked")
        }

    @Test
    fun `nothing is asked for when nothing is picked`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)

            presenter.completeSelected()
            presenter.deleteSelected()
            runCurrent()

            assertTrue(bulk.calls.isEmpty())
        }

    @Test
    fun `a list wired without the actions offers none, and picking still works`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync)
            backgroundScope.launch { presenter.state.collect {} }

            presenter.startSelection(1)
            presenter.completeSelected()
            runCurrent()

            assertFalse(presenter.offersBulkActions)
            assertTrue(presenter.state.value.selectionMode)
            assertTrue(bulk.calls.isEmpty())
        }

    @Test
    fun `a block can be picked and un-picked whole, without disturbing the rest`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            backgroundScope.launch { presenter.state.collect {} }
            presenter.startSelection(7)
            runCurrent()

            presenter.toggleSelectAll(listOf(1L, 2L, 3L))
            runCurrent()
            assertEquals(setOf(7L, 1L, 2L, 3L), presenter.state.value.selected)

            presenter.toggleSelectAll(listOf(1L, 2L, 3L))
            runCurrent()
            assertEquals(setOf(7L), presenter.state.value.selected, "the block goes, the rest stays")

            presenter.toggleSelectAll(emptyList())
            runCurrent()
            assertEquals(setOf(7L), presenter.state.value.selected, "an emptied block picks nothing")
        }

    @Test
    fun `un-picking the last block leaves selection mode`() =
        runTest {
            val presenter = TaskListPresenter(backgroundScope, sections, rowActions, sync, bulk = bulk)
            backgroundScope.launch { presenter.state.collect {} }

            presenter.toggleSelectAll(listOf(1L, 2L))
            runCurrent()
            assertTrue(presenter.state.value.selectionMode)

            presenter.toggleSelectAll(listOf(1L, 2L))
            runCurrent()
            assertFalse(presenter.state.value.selectionMode)
        }
}
