package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.Label
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What one label's own screen does.
 *
 * The work carrying the label is a task list like any other and is checked where
 * task lists are; what is asked here is only what happens to the label itself.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class LabelDetailPresenterTest {
    private val label = MutableStateFlow<Label?>(null)
    private val taggedTasks = MutableStateFlow(0)
    private val actions = RecordingLabelActions()

    private fun TestScope.presenter(
        actions: LabelActions = this@LabelDetailPresenterTest.actions,
    ): LabelDetailPresenter {
        val presenter = LabelDetailPresenter(backgroundScope, label, taggedTasks, actions)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    @Test
    fun `a label not read yet is not the same as one that is gone`() =
        runTest {
            val presenter = presenter()

            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.missing, "nothing has been read, so nothing is known to be missing")
        }

    @Test
    fun `a label deleted underneath the screen leaves it with nothing to show`() =
        runTest {
            val presenter = presenter()
            label.value = label(7, "urgent")
            runCurrent()
            assertFalse(presenter.state.value.missing)

            label.value = null
            runCurrent()

            assertTrue(presenter.state.value.missing)
        }

    @Test
    fun `renaming and recolouring travel as one change`() =
        runTest {
            val presenter = presenter()
            label.value = label(7, "urgent", color = "red")
            runCurrent()

            presenter.save("  important  ", "blue")
            runCurrent()

            assertEquals(listOf("edit(7,important,blue,null,null)"), actions.calls)
        }

    @Test
    fun `a label with no name is not written down`() =
        runTest {
            val presenter = presenter()
            label.value = label(7)
            runCurrent()

            presenter.save("   ", "blue")
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `marking and unmarking are the opposite of what the label says now`() =
        runTest {
            val presenter = presenter()
            label.value = label(7, isFavourite = false, isPrivate = true)
            runCurrent()

            presenter.toggleFavourite()
            presenter.togglePrivate()
            runCurrent()

            assertEquals(
                listOf("edit(7,null,null,true,null)", "edit(7,null,null,null,false)"),
                actions.calls,
            )
        }

    @Test
    fun `the delete warning names how many tasks are about to lose the label`() =
        runTest {
            val presenter = presenter()
            label.value = label(7)
            taggedTasks.value = 40
            runCurrent()

            assertEquals(40, presenter.state.value.taggedTasks)

            presenter.delete()
            runCurrent()

            assertEquals(listOf("delete(7)"), actions.calls)
        }

    @Test
    fun `nothing is written before the label has been read`() =
        runTest {
            val presenter = presenter()

            presenter.save("name", "red")
            presenter.toggleFavourite()
            presenter.togglePrivate()
            presenter.delete()
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `a refused write is reported rather than thrown past the screen`() =
        runTest {
            val refusing = RecordingLabelActions(refuse = true)
            val presenter = presenter(refusing)
            val said = mutableListOf<LabelMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            label.value = label(7)
            runCurrent()

            presenter.delete()
            runCurrent()

            assertEquals(listOf(LabelMessage.FAILED), said)
        }
}
