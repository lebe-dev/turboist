package ru.tinyops.turboist.nativeapp.troiki

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.projects.CountingScheduler
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.projects.RecordingTaskActions
import ru.tinyops.turboist.nativeapp.projects.project
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * What the daily plan does when the user touches it.
 *
 * Two things are worth checking hardest, because both would only be noticed
 * hours later if they were wrong: that a bucket the device already knows to be
 * full refuses the project while the user is still looking at the picker, and
 * that the plan's writes are the plan's — putting a project into a bucket,
 * taking it out, beginning and ending a cycle — while the work inside it goes
 * through the same port every other list uses.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TroikiPresenterTest {
    private val slots = MutableStateFlow<List<TroikiSlot>>(emptyList())
    private val assignable = MutableStateFlow<List<Project>>(emptyList())
    private val actions = RecordingTroikiActions()
    private val taskActions = RecordingTaskActions()
    private val sync = CountingScheduler()

    private fun TestScope.presenter(): TroikiPresenter {
        val presenter =
            TroikiPresenter(backgroundScope, slots, assignable, actions, taskActions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    private fun plan(vararg projects: Project): List<TroikiSlot> = troikiSlots(projects.toList(), emptyList())

    @Test
    fun `a plan not read yet is not the same as a plan holding nothing`() =
        runTest {
            val presenter = presenter()
            assertTrue(presenter.state.value.loading)

            slots.value = plan()
            runCurrent()

            assertTrue(presenter.state.value.isEmpty, "the query answered, and it answered nothing")
        }

    @Test
    fun `the plan carries its three buckets and the projects standing in them`() =
        runTest {
            val presenter = presenter()
            slots.value = plan(project(7, "Website", troikiCategory = TroikiCategory.IMPORTANT))
            runCurrent()

            val state = presenter.state.value
            assertEquals(3, state.slots.size)
            assertEquals(listOf("Website"), state.slots.first().projects.map { it.project.title })
        }

    @Test
    fun `putting a project into a bucket asks for that bucket`() =
        runTest {
            val presenter = presenter()
            presenter.assign(projectLocalId = 7, category = TroikiCategory.MEDIUM)
            runCurrent()

            assertEquals(listOf("setCategory 7 medium"), actions.calls)
        }

    @Test
    fun `taking a project out of the plan clears its bucket rather than naming another`() =
        runTest {
            val presenter = presenter()
            presenter.remove(projectLocalId = 7)
            runCurrent()

            assertEquals(listOf("setCategory 7 null"), actions.calls)
        }

    @Test
    fun `a bucket that is already full refuses the project, with what it holds`() =
        runTest {
            val presenter = presenter()
            actions.refuseWith = WriteRefused.TroikiSlotFull(TroikiCategory.IMPORTANT, capacity = 3)
            val heard = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }

            presenter.assign(projectLocalId = 7, category = TroikiCategory.IMPORTANT)
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.TroikiSlotFull(3)), heard)
        }

    @Test
    fun `work that something still stands in the way of cannot be ticked off here either`() =
        runTest {
            val presenter = presenter()
            taskActions.refuseWith = WriteRefused.TaskBlocked(listOf(9))
            val heard = mutableListOf<ProjectMessage>()
            backgroundScope.launch { presenter.messages.collect { heard += it } }

            presenter.toggleComplete(task(5))
            runCurrent()

            assertEquals(listOf<ProjectMessage>(ProjectMessage.Blocked), heard)
        }

    @Test
    fun `ticking a task off, and putting it back, go through the port every list uses`() =
        runTest {
            val presenter = presenter()

            presenter.toggleComplete(task(5))
            presenter.toggleComplete(task(6, status = TaskStatus.COMPLETED, completedAt = 1))
            runCurrent()

            assertEquals(listOf("complete 5", "uncomplete 6"), taskActions.calls)
            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `both controls of the cycle are available, because the device cannot know which is right`() =
        runTest {
            val presenter = presenter()

            presenter.start()
            presenter.reset()
            runCurrent()

            assertEquals(listOf("start", "reset"), actions.calls)
        }

    @Test
    fun `a task written down with a blank title is not written down at all`() =
        runTest {
            val presenter = presenter()

            presenter.addTask(projectLocalId = 7, title = "   ")
            presenter.addTask(projectLocalId = 7, title = "  Call the plumber  ")
            runCurrent()

            assertEquals(listOf("addTask 7 Call the plumber"), actions.calls)
        }

    @Test
    fun `pulling the plan down asks the sync engine to catch up rather than reloading it`() =
        runTest {
            val presenter = presenter()

            presenter.refresh()
            runCurrent()

            assertEquals(1, sync.requests)
        }
}
