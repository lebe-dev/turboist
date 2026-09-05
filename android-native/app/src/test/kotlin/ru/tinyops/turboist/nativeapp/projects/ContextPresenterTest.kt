package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the screen showing one context does with the branch it is given.
 *
 * A context is a place rather than a list, so the screen answers with both
 * halves of what is under it — the projects and the work — and the checks here
 * are about that shape and about the two writes the screen makes.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ContextPresenterTest {
    private val content = MutableStateFlow<ContextContent?>(null)
    private val actions = RecordingContextActions()
    private val taskActions = RecordingTaskActions()
    private val sync = CountingScheduler()

    private fun kotlinx.coroutines.test.TestScope.presenter(): ContextPresenter {
        val presenter = ContextPresenter(backgroundScope, content, MutableStateFlow(true), actions, taskActions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    @Test
    fun `a context not read yet is not the same as a context that is gone`() =
        runTest {
            val presenter = presenter()
            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.missing)

            content.value = null
            runCurrent()

            assertTrue(presenter.state.value.missing)
        }

    @Test
    fun `the branch carries its projects and every task under it`() =
        runTest {
            val presenter = presenter()
            content.value =
                ContextContent(
                    context = context(1, "Work"),
                    projects = listOf(project(10, "Website", contextLocalId = 1)),
                    tasks = listOf(task(5, projectLocalId = 10), task(6)),
                )
            runCurrent()

            assertEquals(listOf("Website"), presenter.state.value.projects.map { it.title })
            assertEquals(listOf(5L, 6L), presenter.state.value.rows.map { it.task.localId })
            assertEquals(
                "Website",
                presenter.state.value.rows.first().projectTitle,
                "a row inside a project says which one",
            )
            assertFalse(presenter.state.value.isEmpty)
        }

    @Test
    fun `a context holding nothing at all says so`() =
        runTest {
            val presenter = presenter()
            content.value = ContextContent(context(1), emptyList(), emptyList())
            runCurrent()

            assertTrue(presenter.state.value.isEmpty)
        }

    @Test
    fun `renaming trims what was typed and ignores an empty name`() =
        runTest {
            val presenter = presenter()
            content.value = ContextContent(context(1), emptyList(), emptyList())
            runCurrent()

            presenter.rename("  Home  ")
            presenter.rename("   ")
            runCurrent()

            assertEquals(listOf("renameContext 1 Home"), actions.calls)
        }

    @Test
    fun `deleting names the context the screen is showing`() =
        runTest {
            val presenter = presenter()
            content.value = ContextContent(context(7), emptyList(), emptyList())
            runCurrent()

            presenter.delete()
            runCurrent()

            assertEquals(listOf("deleteContext 7"), actions.calls)
        }
}
