package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.WriteRefused
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
    ) = ProjectBoardContent(
        project = project(1, "Website", status = status),
        contextName = "Work",
        sections = sections,
        tasks = tasks,
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
}
