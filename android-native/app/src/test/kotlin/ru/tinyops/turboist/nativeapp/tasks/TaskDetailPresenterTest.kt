package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.templates.TemplateCapture
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the task detail screen does when the user touches it.
 *
 * Two claims are checked over and over. Every write is applied without waiting
 * for a server, so nothing here has a network in it at all; and every field
 * editor sends exactly the field that changed, because a write carrying more
 * than that would overwrite whatever another device changed in the meantime.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TaskDetailPresenterTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val content = MutableStateFlow<TaskDetailContent?>(null)
    private val actions = RecordingActions()
    private val sync = CountingScheduler()
    private val relationSearch = StubRelationSearch()

    /** Every write the screen can make, remembered rather than performed. */
    private class RecordingActions : TaskDetailActions {
        val calls = mutableListOf<String>()
        val edits = mutableListOf<TaskEdit>()
        var refuseWith: WriteRefused? = null

        private fun record(call: String) {
            calls += call
            refuseWith?.let { throw it }
        }

        override suspend fun complete(taskLocalId: Long) = record("complete $taskLocalId")

        override suspend fun uncomplete(taskLocalId: Long) = record("uncomplete $taskLocalId")

        override suspend fun edit(
            taskLocalId: Long,
            edit: TaskEdit,
        ) {
            edits += edit
            record("edit $taskLocalId")
        }

        override suspend fun cancel(taskLocalId: Long) = record("cancel $taskLocalId")

        override suspend fun pin(taskLocalId: Long) = record("pin $taskLocalId")

        override suspend fun unpin(taskLocalId: Long) = record("unpin $taskLocalId")

        override suspend fun duplicate(taskLocalId: Long) = record("duplicate $taskLocalId")

        override suspend fun decompose(
            taskLocalId: Long,
            titles: List<String>,
        ) = record("decompose $taskLocalId ${titles.joinToString("|")}")

        override suspend fun plan(
            taskLocalId: Long,
            state: PlanState,
        ) = record("plan $taskLocalId ${state.wire}")

        override suspend fun move(
            taskLocalId: Long,
            destination: TaskDestination,
        ) = record("move $taskLocalId $destination")

        override suspend fun delete(taskLocalId: Long) = record("delete $taskLocalId")

        override suspend fun createSubtask(
            parentTaskLocalId: Long,
            title: String,
        ) = record("subtask $parentTaskLocalId $title")

        override suspend fun addRelation(
            taskLocalId: Long,
            peerTaskLocalId: Long,
            group: TaskRelationGroup,
        ) = record("addRelation $taskLocalId $peerTaskLocalId $group")

        override suspend fun removeRelation(
            taskLocalId: Long,
            relationLocalId: Long,
        ) = record("removeRelation $taskLocalId $relationLocalId")
    }

    /** A device that answers the picker with whatever a test put in it. */
    private class StubRelationSearch : TaskRelationSearch {
        var answer: List<TaskRelationCandidate> = emptyList()
        val asked = mutableListOf<String>()

        override suspend fun candidates(typed: String): List<TaskRelationCandidate> {
            asked += typed
            return answer
        }
    }

    /** A sync engine nobody is supposed to need for any of this. */
    private class CountingScheduler : SyncScheduler {
        var requests = 0

        override suspend fun requestSyncNow() {
            requests++
        }
    }

    private fun detail(
        task: Task,
        subtasks: List<Task> = emptyList(),
        blockers: List<BlockerRef> = emptyList(),
        relations: List<TaskRelationRef> = emptyList(),
        placement: TaskPlacement = TaskPlacement(),
        knownLabels: List<Label> = emptyList(),
        priorityLockedByTroiki: Boolean = false,
    ) = TaskDetailContent(
        task = task,
        placement = placement,
        blockers = blockers,
        relations = relations,
        subtasks = subtasks,
        projectTitles = emptyMap(),
        knownLabels = knownLabels,
        priorityLockedByTroiki = priorityLockedByTroiki,
    )

    private fun link(
        peerLocalId: Long,
        group: TaskRelationGroup = TaskRelationGroup.BLOCKED_BY,
        title: String = "Order the parts",
        status: TaskStatus = TaskStatus.OPEN,
    ) = TaskRelationRef(
        relationLocalId = peerLocalId,
        group = group,
        peerLocalId = peerLocalId,
        peerServerId = peerLocalId,
        peerTitle = title,
        peerStatus = status,
    )

    private fun candidate(
        taskLocalId: Long,
        title: String = "Order the parts",
    ) = TaskRelationCandidate(
        taskLocalId = taskLocalId,
        serverId = taskLocalId,
        title = title,
        status = TaskStatus.OPEN,
        projectTitle = null,
    )

    @Test
    fun `the state carries the task it was given, and is no longer loading`() =
        runTest {
            val presenter = TaskDetailPresenter(backgroundScope, content, actions, sync)
            backgroundScope.launch { presenter.state.collect {} }
            content.value = detail(task(1, title = "Buy milk"))
            runCurrent()

            assertFalse(presenter.state.value.loading)
            assertEquals("Buy milk", presenter.state.value.task?.title)
            assertFalse(presenter.state.value.missing)
        }

    @Test
    fun `a task the replica does not hold is missing rather than merely unread`() =
        runTest {
            val presenter = TaskDetailPresenter(backgroundScope, content, actions, sync)
            assertFalse(presenter.state.value.missing, "nothing has been read yet, so nothing is known to be absent")

            backgroundScope.launch { presenter.state.collect {} }
            content.value = null
            runCurrent()

            assertTrue(presenter.state.value.missing)
        }

    @Test
    fun `renaming sends the title and nothing else`() =
        runTest {
            val presenter = open(task(1, title = "old", description = "kept", priority = Priority.HIGH))

            presenter.rename("new")
            runCurrent()

            assertEquals(listOf("title"), actions.edits.single().touchedFields())
            assertEquals("new", actions.edits.single().title)
        }

    @Test
    fun `each field editor sends one field`() =
        runTest {
            val presenter =
                open(task(1, title = "t", description = "d", priority = Priority.LOW, dayPart = DayPart.NONE))

            presenter.describe("longer")
            presenter.setPriority(Priority.HIGH)
            presenter.setDayPart(DayPart.MORNING)
            presenter.setComplex(true)
            presenter.setPrivate(true)
            presenter.setDueDate(LocalDate.of(2026, 3, 12), zone)
            runCurrent()

            assertEquals(
                listOf(
                    listOf("description"),
                    listOf("priority"),
                    listOf("dayPart"),
                    listOf("isComplex"),
                    listOf("isPrivate"),
                    listOf("dueAt"),
                ),
                actions.edits.map { it.touchedFields() },
            )
        }

    @Test
    fun `a priority the project already decides is neither applied nor queued`() =
        runTest {
            val presenter =
                open(task(1, title = "In the plan", priority = Priority.HIGH), priorityLockedByTroiki = true)

            presenter.setPriority(Priority.LOW)
            runCurrent()

            assertTrue(presenter.state.value.priorityLocked, "the screen has to say why the field is not offered")
            assertTrue(actions.edits.isEmpty(), "the server refuses this edit, so nothing may be queued")
            assertEquals(Priority.HIGH, presenter.state.value.task?.priority)
        }

    @Test
    fun `an editor handed the value the task already has sends nothing`() =
        runTest {
            val presenter =
                open(
                    task(
                        1,
                        title = "same",
                        description = "same too",
                        priority = Priority.HIGH,
                        dayPart = DayPart.EVENING,
                        isComplex = true,
                        isPrivate = true,
                    ),
                )

            presenter.rename("same")
            presenter.describe("same too")
            presenter.setPriority(Priority.HIGH)
            presenter.setDayPart(DayPart.EVENING)
            presenter.setComplex(true)
            presenter.setPrivate(true)
            presenter.setLabels(emptyList())
            runCurrent()

            assertTrue(actions.edits.isEmpty(), "nothing changed, so nothing should have been queued")
            assertTrue(actions.calls.isEmpty())
        }

    @Test
    fun `a title trimmed to nothing is refused rather than sent`() =
        runTest {
            val presenter = open(task(1, title = "Buy milk"))

            presenter.rename("   ")
            runCurrent()

            assertTrue(actions.edits.isEmpty())
        }

    @Test
    fun `labels are sent as the whole set, and only when the set changed`() =
        runTest {
            val urgent = Label(localId = 7, name = "urgent", createdAt = 0, updatedAt = 0)
            val presenter = open(task(1, labels = listOf(urgent)))

            presenter.setLabels(listOf("urgent"))
            runCurrent()
            assertTrue(actions.edits.isEmpty())

            presenter.setLabels(listOf("urgent", "home"))
            runCurrent()
            assertEquals(listOf("labels"), actions.edits.single().touchedFields())
            assertEquals(listOf("urgent", "home"), actions.edits.single().labels)
        }

    @Test
    fun `planning is its own write, not a field edit`() =
        runTest {
            // The plan state carries the open work beneath the task with it, which
            // a patch of one column would not.
            val presenter = open(task(1))

            presenter.setPlanState(PlanState.BACKLOG)
            runCurrent()

            assertTrue(actions.edits.isEmpty())
            assertEquals(listOf("plan 1 backlog"), actions.calls)
        }

    @Test
    fun `ticking an open task completes it, and ticking a finished one puts it back`() =
        runTest {
            val presenter = open(task(1))
            presenter.toggleComplete()
            runCurrent()
            assertEquals(listOf("complete 1"), actions.calls)

            content.value = detail(task(1, status = TaskStatus.COMPLETED, completedAt = 5L))
            runCurrent()
            presenter.toggleComplete()
            runCurrent()
            assertEquals(listOf("complete 1", "uncomplete 1"), actions.calls)
        }

    @Test
    fun `a refused completion is reported as blocked rather than thrown`() =
        runTest {
            val presenter = open(task(1))
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.TaskBlocked(listOf(2L))

            presenter.toggleComplete()
            runCurrent()

            assertEquals(listOf(TaskListMessage.BLOCKED), said)
        }

    @Test
    fun `any other refusal says only that the change did not happen`() =
        runTest {
            val presenter = open(task(1))
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.PinLimitReached(10)

            presenter.togglePin()
            runCurrent()

            assertEquals(listOf(TaskListMessage.FAILED), said)
        }

    @Test
    fun `a task with open blockers reads as blocked until it is finished`() =
        runTest {
            val presenter =
                open(task(1), blockers = listOf(BlockerRef(taskLocalId = 2, serverId = 2, title = "Order parts")))
            assertTrue(presenter.state.value.blocked)

            content.value =
                detail(
                    task(1, status = TaskStatus.COMPLETED),
                    blockers = listOf(BlockerRef(taskLocalId = 2, serverId = 2, title = "Order parts")),
                )
            runCurrent()

            assertFalse(presenter.state.value.blocked, "a finished task cannot be waiting on anything")
        }

    @Test
    fun `pinning and unpinning follow the state the task is in`() =
        runTest {
            val presenter = open(task(1, isPinned = true))
            presenter.togglePin()
            runCurrent()

            assertEquals(listOf("unpin 1"), actions.calls)
        }

    @Test
    fun `deleting the task tells the screen there is nothing left to show`() =
        runTest {
            val presenter = open(task(1))
            var left = 0
            backgroundScope.launch { presenter.deleted.collect { left++ } }

            presenter.delete()
            runCurrent()

            assertEquals(listOf("delete 1"), actions.calls)
            assertEquals(1, left)
        }

    @Test
    fun `a delete the write path refuses leaves the screen where it is`() =
        runTest {
            val presenter = open(task(1))
            var left = 0
            backgroundScope.launch { presenter.deleted.collect { left++ } }
            actions.refuseWith = WriteRefused.RowMissing("task", 1)

            presenter.delete()
            runCurrent()

            assertEquals(0, left)
        }

    @Test
    fun `an empty subtask title is not written down`() =
        runTest {
            val presenter = open(task(1))

            presenter.addSubtask("  ")
            presenter.addSubtask(" Wash up ")
            runCurrent()

            assertEquals(listOf("subtask 1 Wash up"), actions.calls)
        }

    @Test
    fun `finished subtasks are kept apart from the open ones`() =
        runTest {
            val presenter =
                open(
                    task(1),
                    subtasks =
                        listOf(
                            task(2, parentLocalId = 1),
                            task(3, parentLocalId = 1, status = TaskStatus.COMPLETED),
                        ),
                )

            assertEquals(listOf(2L), presenter.state.value.openSubtasks.map { it.task.localId })
            assertEquals(listOf(3L), presenter.state.value.doneSubtasks.map { it.task.localId })
        }

    @Test
    fun `moving the task hands the destination straight to the write path`() =
        runTest {
            val presenter = open(task(1))

            presenter.move(TaskDestination.InSection(9))
            runCurrent()

            assertEquals(listOf("move 1 ${TaskDestination.InSection(9)}"), actions.calls)
        }

    @Test
    fun `every write is made without asking the sync engine for anything`() =
        runTest {
            val presenter = open(task(1))

            presenter.rename("changed")
            presenter.setPriority(Priority.HIGH)
            presenter.toggleComplete()
            presenter.duplicate()
            runCurrent()

            assertEquals(0, sync.requests, "the screen must work with no network at all")
        }

    @Test
    fun `pulling the screen down asks the sync engine to catch up`() =
        runTest {
            val presenter = open(task(1))

            presenter.refresh()
            runCurrent()

            assertEquals(1, sync.requests)
        }

    // --- links to other tasks ------------------------------------------

    @Test
    fun `the links the task carries reach the screen grouped by what they say`() =
        runTest {
            val presenter =
                open(
                    task(1),
                    relations =
                        listOf(
                            link(2, TaskRelationGroup.BLOCKED_BY),
                            link(3, TaskRelationGroup.BLOCKS, title = "Ship it"),
                            link(4, TaskRelationGroup.RELATED, title = "Read the spec"),
                        ),
                )

            val peersIn = { group: TaskRelationGroup ->
                presenter.state.value.relationsIn(group).map { it.peerLocalId }
            }
            assertEquals(listOf(2L), peersIn(TaskRelationGroup.BLOCKED_BY))
            assertEquals(listOf(3L), peersIn(TaskRelationGroup.BLOCKS))
            assertEquals(listOf(4L), peersIn(TaskRelationGroup.RELATED))
        }

    @Test
    fun `adding a link names the peer and what the link says`() =
        runTest {
            val presenter = open(task(1))

            presenter.addRelation(7, TaskRelationGroup.BLOCKED_BY)
            runCurrent()

            assertEquals(listOf("addRelation 1 7 BLOCKED_BY"), actions.calls)
        }

    @Test
    fun `the picker never offers the task itself or a task already linked`() =
        runTest {
            val presenter = open(task(1), relations = listOf(link(2)))
            relationSearch.answer = listOf(candidate(1), candidate(2), candidate(3, title = "Free"))

            presenter.searchRelationCandidates("or")
            runCurrent()

            // Both would be turned down by the write path, so neither is put in
            // front of the user in the first place.
            assertEquals(listOf(3L), presenter.state.value.relationCandidates.map { it.taskLocalId })
        }

    @Test
    fun `an empty search offers nothing and asks the device nothing`() =
        runTest {
            val presenter = open(task(1))
            relationSearch.answer = listOf(candidate(3))
            presenter.searchRelationCandidates("or")
            runCurrent()

            presenter.searchRelationCandidates("  ")
            runCurrent()

            assertTrue(presenter.state.value.relationCandidates.isEmpty())
            assertEquals(listOf("or"), relationSearch.asked)
        }

    @Test
    fun `a link that was made empties the picker behind it`() =
        runTest {
            val presenter = open(task(1))
            relationSearch.answer = listOf(candidate(7))
            presenter.searchRelationCandidates("or")
            runCurrent()

            presenter.addRelation(7, TaskRelationGroup.RELATED)
            runCurrent()

            assertTrue(presenter.state.value.relationCandidates.isEmpty())
        }

    @Test
    fun `a link that is already there is reported as such rather than as a failure`() =
        runTest {
            val presenter = open(task(1))
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.RelationExists(4)

            presenter.addRelation(7, TaskRelationGroup.RELATED)
            runCurrent()

            assertEquals(listOf(TaskListMessage.RELATION_EXISTS), said)
        }

    @Test
    fun `a link that would leave both tasks waiting is reported as such`() =
        runTest {
            val presenter = open(task(1))
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.RelationCycle(blockerLocalId = 7, blockedLocalId = 1)

            presenter.addRelation(7, TaskRelationGroup.BLOCKED_BY)
            runCurrent()

            assertEquals(listOf(TaskListMessage.RELATION_CYCLE), said)
        }

    @Test
    fun `removing a link names the task it is being removed from`() =
        runTest {
            val presenter = open(task(1), relations = listOf(link(2)))

            presenter.removeRelation(2)
            runCurrent()

            assertEquals(listOf("removeRelation 1 2"), actions.calls)
        }

    @Test
    fun `linking asks the sync engine for nothing`() =
        runTest {
            val presenter = open(task(1))
            relationSearch.answer = listOf(candidate(7))

            presenter.searchRelationCandidates("or")
            presenter.addRelation(7, TaskRelationGroup.RELATED)
            presenter.removeRelation(2)
            runCurrent()

            assertEquals(0, sync.requests, "the whole of it is answered from the device")
        }

    @Test
    fun `splitting a task asks for the titles that were typed, trimmed and without the blank lines`() =
        runTest {
            val presenter = open(task(1, title = "Draft outline"))

            presenter.decompose(listOf("  Review with team ", "", "   ", "Publish"))
            runCurrent()

            assertEquals(listOf("decompose 1 Review with team|Publish"), actions.calls)
        }

    @Test
    fun `an outline with nothing in it asks for nothing`() =
        runTest {
            val presenter = open(task(1, title = "Draft outline"))

            presenter.decompose(listOf("   ", ""))
            runCurrent()

            assertEquals(emptyList(), actions.calls, "a split into no tasks would only delete the task")
        }

    @Test
    fun `a split the device refuses is reported in the split's own words`() =
        runTest {
            val presenter = open(task(1, title = "Draft outline"))
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.Invalid("a task with subtasks cannot be split")

            presenter.decompose(listOf("Review with team"))
            runCurrent()

            assertEquals(listOf(TaskListMessage.DECOMPOSE_FAILED), said)
        }

    @Test
    fun `cutting a template out of the task says so, because nothing on this screen changes`() =
        runTest {
            val capture = RecordingCapture()
            val presenter =
                TaskDetailPresenter(backgroundScope, content, actions, sync, relationSearch, capture)
            backgroundScope.launch { presenter.state.collect {} }
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            content.value = detail(task(1, title = "Ship the site"))
            runCurrent()

            presenter.createTemplate()
            runCurrent()

            assertEquals(listOf(1L), capture.captured)
            assertEquals(listOf(TaskListMessage.TEMPLATE_CREATED), said)
        }

    @Test
    fun `a template that could not be cut says that instead`() =
        runTest {
            val capture = RecordingCapture().apply { refuseWith = WriteRefused.RowMissing("task", 1) }
            val presenter =
                TaskDetailPresenter(backgroundScope, content, actions, sync, relationSearch, capture)
            backgroundScope.launch { presenter.state.collect {} }
            val said = mutableListOf<TaskListMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            content.value = detail(task(1, title = "Ship the site"))
            runCurrent()

            presenter.createTemplate()
            runCurrent()

            assertEquals(listOf(TaskListMessage.TEMPLATE_FAILED), said)
        }

    /** Cutting a template out of a task, remembered rather than performed. */
    private class RecordingCapture : TemplateCapture {
        val captured = mutableListOf<Long>()
        var refuseWith: WriteRefused? = null

        override suspend fun captureFromTask(taskLocalId: Long) {
            refuseWith?.let { throw it }
            captured += taskLocalId
        }
    }

    /** Builds a presenter, starts watching it, and points it at one task. */
    private fun kotlinx.coroutines.test.TestScope.open(
        task: Task,
        subtasks: List<Task> = emptyList(),
        blockers: List<BlockerRef> = emptyList(),
        relations: List<TaskRelationRef> = emptyList(),
        priorityLockedByTroiki: Boolean = false,
    ): TaskDetailPresenter {
        val presenter = TaskDetailPresenter(backgroundScope, content, actions, sync, relationSearch)
        backgroundScope.launch { presenter.state.collect {} }
        content.value =
            detail(
                task,
                subtasks = subtasks,
                blockers = blockers,
                relations = relations,
                priorityLockedByTroiki = priorityLockedByTroiki,
            )
        runCurrent()
        return presenter
    }
}
