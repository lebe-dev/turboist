package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.view.BlockEdge
import ru.tinyops.turboist.core.model.view.DependencyDropRefusal
import ru.tinyops.turboist.core.model.view.NestDropRefusal
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.tasks.DependencyAdded
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the project board does when the user touches it.
 *
 * The two things worth checking hardest are the ones a user would notice going
 * wrong hours later: which position a column move asks for, and which placement
 * a task move asks for. Both are computed from the board on screen, and both are
 * sent to the server unchanged.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ProjectBoardPresenterTest {
    private val content = MutableStateFlow<ProjectBoardContent?>(null)
    private val dailyPlanEnabled = MutableStateFlow(true)
    private val actions = RecordingProjectActions()
    private val taskActions = RecordingTaskActions()
    private val sync = CountingScheduler()

    private fun kotlinx.coroutines.test.TestScope.presenter(): ProjectBoardPresenter {
        val presenter = ProjectBoardPresenter(backgroundScope, content, dailyPlanEnabled, actions, taskActions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    private fun board(
        sections: List<ru.tinyops.turboist.core.model.ProjectSection> = emptyList(),
        tasks: List<ru.tinyops.turboist.core.model.Task> = emptyList(),
        status: ProjectStatus = ProjectStatus.OPEN,
        blockEdges: List<BlockEdge> = emptyList(),
    ) = ProjectBoardContent(
        project = project(1, "Website", status = status),
        contextName = "Work",
        sections = sections,
        tasks = tasks,
        blockEdges = blockEdges,
    )

    @Test
    fun `a project not read yet is not the same as a project that is gone`() =
        runTest {
            val presenter = presenter()
            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.missing)

            content.value = null
            runCurrent()

            assertTrue(presenter.state.value.missing, "the query answered, and it answered nothing")
        }

    @Test
    fun `the board carries the project, its context and its columns`() =
        runTest {
            val presenter = presenter()
            content.value = board(sections = listOf(section(10, "Doing")), tasks = listOf(task(1)))
            runCurrent()

            assertEquals("Website", presenter.state.value.project?.title)
            assertEquals("Work", presenter.state.value.contextName)
            assertEquals(listOf(null, 10L), presenter.state.value.columns.map { it.sectionLocalId })
        }

    @Test
    fun `moving a column one place asks for the position it will end up at`() =
        runTest {
            val presenter = presenter()
            content.value =
                board(
                    sections =
                        listOf(
                            section(10, "Backlog", position = 0),
                            section(11, "Doing", position = 1),
                            section(12, "Done", position = 2),
                        ),
                )
            runCurrent()

            presenter.moveSectionLater(sectionLocalId = 10)
            presenter.moveSectionEarlier(sectionLocalId = 12)
            runCurrent()

            assertEquals(listOf("reorderSection 10 1", "reorderSection 12 1"), actions.calls)
        }

    @Test
    fun `a column at the end of the board is not asked to move past it`() =
        runTest {
            val presenter = presenter()
            content.value = board(sections = listOf(section(10, position = 0)))
            runCurrent()
            val only = presenter.state.value.columns.last()

            assertFalse(only.canMoveEarlier)
            assertFalse(only.canMoveLater)
        }

    @Test
    fun `writing a task down in a column names that column, and in none names the project`() =
        runTest {
            val presenter = presenter()
            content.value = board(sections = listOf(section(10)))
            runCurrent()

            presenter.addTask(sectionLocalId = 10, title = "  Ship it  ")
            presenter.addTask(sectionLocalId = null, title = "Think about it")
            runCurrent()

            assertEquals(
                listOf(
                    "addTask InSection(sectionLocalId=10) Ship it",
                    "addTask InProject(projectLocalId=1) Think about it",
                ),
                actions.calls,
            )
        }

    @Test
    fun `a task with no title is not written down at all`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()

            presenter.addTask(sectionLocalId = null, title = "  ")
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `moving a task out of every column puts it back in the project itself`() =
        runTest {
            val presenter = presenter()
            content.value = board(sections = listOf(section(10)), tasks = listOf(task(5, sectionLocalId = 10)))
            runCurrent()

            presenter.moveTask(taskLocalId = 5, sectionLocalId = null)
            presenter.moveTask(taskLocalId = 5, sectionLocalId = 10)
            runCurrent()

            assertEquals(
                listOf("moveTask 5 InProject(projectLocalId=1)", "moveTask 5 InSection(sectionLocalId=10)"),
                actions.calls,
            )
        }

    @Test
    fun `ticking a row off completes it, and ticking a finished one puts it back`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()

            presenter.toggleComplete(task(5))
            presenter.toggleComplete(task(6, status = TaskStatus.COMPLETED, completedAt = 1))
            runCurrent()

            assertEquals(listOf("complete 5", "uncomplete 6"), taskActions.calls)
        }

    @Test
    fun `work something still blocks says so instead of pretending`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()
            taskActions.refuseWith = WriteRefused.TaskBlocked(listOf(9))
            val heard = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }

            presenter.toggleComplete(task(5))
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.Blocked), heard)
        }

    @Test
    fun `a full shelf is reported with the size of the shelf`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()
            actions.refuseWith = WriteRefused.PinLimitReached(limit = 10)
            val heard = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }

            presenter.togglePin()
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.PinLimitReached(10)), heard)
        }

    @Test
    fun `a full slot of the daily plan is reported with what the slot holds`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()
            actions.refuseWith = WriteRefused.TroikiSlotFull(TroikiCategory.IMPORTANT, capacity = 3)
            val heard = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }

            presenter.setTroikiCategory(TroikiCategory.IMPORTANT)
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.TroikiSlotFull(3)), heard)
        }

    @Test
    fun `only an open project is offered a place in the daily plan`() =
        runTest {
            val presenter = presenter()
            content.value = board(status = ProjectStatus.OPEN)
            runCurrent()
            assertTrue(presenter.state.value.canJoinDailyPlan)

            content.value = board(status = ProjectStatus.COMPLETED)
            runCurrent()

            assertFalse(presenter.state.value.canJoinDailyPlan)
        }

    @Test
    fun `pinning a pinned project unpins it`() =
        runTest {
            val presenter = presenter()
            content.value =
                ProjectBoardContent(project(1, isPinned = true), "Work", emptyList(), emptyList())
            runCurrent()

            presenter.togglePin()
            runCurrent()

            assertEquals(listOf("unpinProject 1"), actions.calls)
        }

    @Test
    fun `finishing the project asks for the status the action stands for`() =
        runTest {
            val presenter = presenter()
            content.value = board()
            runCurrent()

            presenter.setStatus(ProjectStatusAction.ARCHIVE)
            runCurrent()

            assertEquals(listOf("setProjectStatus 1 archive"), actions.calls)
        }

    @Test
    fun `a column is not created, renamed or moved before the project is known`() =
        runTest {
            val presenter = presenter()

            presenter.createSection("Doing")
            presenter.moveSectionLater(sectionLocalId = 10)
            presenter.addTask(sectionLocalId = null, title = "Ship it")
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    // --- dragging one task onto another -------------------------------------

    /** Two columns: 2 and 3 in the project's own, 3 holding 4; 5 in "Doing". */
    private val dragBoard =
        board(
            sections = listOf(section(10, "Doing")),
            tasks =
                listOf(
                    task(2, title = "Design", projectLocalId = 1),
                    task(3, title = "Build", projectLocalId = 1),
                    task(4, title = "Wire it up", projectLocalId = 1, parentLocalId = 3),
                    task(5, title = "Launch", projectLocalId = 1, sectionLocalId = 10),
                    task(6, title = "Kickoff", projectLocalId = 1, status = TaskStatus.COMPLETED),
                ),
            blockEdges = listOf(BlockEdge(blockerLocalId = 2, blockedLocalId = 5)),
        )

    @Test
    fun `dropping a task on another's edge makes it wait for that one, across columns, and offers an undo`() =
        runTest {
            val presenter = presenter()
            content.value = dragBoard
            runCurrent()
            val added = mutableListOf<DependencyAdded>()
            backgroundScope.launch { presenter.dependencyAdded.collect { added += it } }

            presenter.makeDependent(draggedLocalId = 3, targetLocalId = 5)
            runCurrent()

            assertEquals(listOf("addBlocker 3 5"), actions.calls)
            assertEquals(listOf(DependencyAdded(3, RecordingProjectActions.ADDED_RELATION_LOCAL_ID, "Launch")), added)

            presenter.undoDependency(added.single())
            runCurrent()

            assertEquals("removeRelation 3 ${RecordingProjectActions.ADDED_RELATION_LOCAL_ID}", actions.calls.last())
        }

    @Test
    fun `the board knows while the finger is still down which drops it would refuse`() =
        runTest {
            val presenter = presenter()
            content.value = dragBoard
            runCurrent()
            val state = presenter.state.value

            assertEquals(DependencyDropRefusal.EXISTS, state.dependencyRefusal(draggedLocalId = 5, targetLocalId = 2))
            assertEquals(DependencyDropRefusal.CYCLE, state.dependencyRefusal(draggedLocalId = 2, targetLocalId = 5))
            assertEquals(
                DependencyDropRefusal.COMPLETED,
                state.dependencyRefusal(draggedLocalId = 2, targetLocalId = 6),
            )
            assertEquals(DependencyDropRefusal.ANCESTOR, state.dependencyRefusal(draggedLocalId = 4, targetLocalId = 3))
            assertEquals(NestDropRefusal.DESCENDANT, state.nestRefusal(draggedLocalId = 3, targetLocalId = 4))
            assertTrue(state.nestNoop(draggedLocalId = 4, targetLocalId = 3))
            assertTrue(state.canDetach(4))
            assertFalse(state.canDetach(3))
        }

    @Test
    fun `a refused drop queues nothing`() =
        runTest {
            val presenter = presenter()
            content.value = dragBoard
            runCurrent()

            presenter.makeDependent(draggedLocalId = 2, targetLocalId = 6)
            presenter.nestUnder(draggedLocalId = 3, targetLocalId = 4)
            presenter.nestUnder(draggedLocalId = 4, targetLocalId = 3)
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `dropping a task on another's middle nests it there, whichever column the target is in`() =
        runTest {
            val presenter = presenter()
            content.value = dragBoard
            runCurrent()

            presenter.nestUnder(draggedLocalId = 2, targetLocalId = 5)
            runCurrent()

            assertEquals(listOf("moveTask 2 SubtaskOf(parentTaskLocalId=5)"), actions.calls)
        }

    @Test
    fun `dropping a subtask on the detach zone lifts it to the top of its own column`() =
        runTest {
            val presenter = presenter()
            content.value =
                board(
                    sections = listOf(section(10, "Doing")),
                    tasks =
                        listOf(
                            task(5, projectLocalId = 1, sectionLocalId = 10),
                            task(7, projectLocalId = 1, sectionLocalId = 10, parentLocalId = 5),
                            task(3, projectLocalId = 1),
                        ),
                )
            runCurrent()

            presenter.detach(7)
            presenter.detach(3)
            runCurrent()

            assertEquals(listOf("moveTask 7 InSection(sectionLocalId=10)"), actions.calls)
        }

    @Test
    fun `a link the device turns down is told apart from a plain failure`() =
        runTest {
            val presenter = presenter()
            content.value = dragBoard
            runCurrent()
            val said = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.RelationCycle(blockerLocalId = 5, blockedLocalId = 3)

            presenter.makeDependent(draggedLocalId = 3, targetLocalId = 5)
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.RelationCycle), said)
        }
}
