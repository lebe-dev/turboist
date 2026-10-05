package ru.tinyops.turboist.nativeapp.unsent

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the unsent-changes screen does with the two piles it is given.
 *
 * The contract it exists to keep is that nothing disappears without a person
 * saying so, and that nothing a person could still have lands under an action
 * that throws it away. So the pile that is still going out carries no action at
 * all, and clearing the pile that is not is asked about first.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class UnsentChangesPresenterTest {
    private val setAside = MutableStateFlow<List<UnsentChange>>(emptyList())
    private val waiting = MutableStateFlow<List<UnsentChange>>(emptyList())
    private val actions = RecordingUnsentActions()

    private fun TestScope.presenter(): UnsentChangesPresenter {
        val presenter = UnsentChangesPresenter(backgroundScope, setAside, waiting, actions)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    @Test
    fun `a queue not read yet is not the same as one with nothing in it`() =
        runTest {
            val presenter = presenter()

            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.isEmpty, "nothing has been read, so nothing is known to be empty")
        }

    @Test
    fun `both piles reach the screen`() =
        runTest {
            val presenter = presenter()
            setAside.value = listOf(refused("a", UnsentReason.BLOCKED))
            waiting.value = listOf(queued("b"))
            runCurrent()

            assertEquals(listOf("a"), presenter.state.value.setAside.map { it.id })
            assertEquals(listOf("b"), presenter.state.value.waiting.map { it.id })
            assertFalse(presenter.state.value.isEmpty)
        }

    @Test
    fun `discarding one names the change the queue knows it by`() =
        runTest {
            val presenter = presenter()
            val change = refused("a", UnsentReason.TARGET_GONE)
            setAside.value = listOf(change)
            runCurrent()

            presenter.discard(change)
            runCurrent()

            assertEquals(listOf("a"), actions.discarded)
        }

    @Test
    fun `clearing the whole pile is asked about before it happens`() =
        runTest {
            val presenter = presenter()
            setAside.value = listOf(refused("a", UnsentReason.CONFLICT), refused("b", UnsentReason.INVALID))
            runCurrent()

            presenter.askToDiscardAll()
            runCurrent()

            assertTrue(presenter.state.value.confirmingDiscardAll)
            assertEquals(0, actions.discardedAll, "nothing goes until the question is answered")
        }

    @Test
    fun `answering yes clears the pile and closes the question`() =
        runTest {
            val presenter = presenter()
            setAside.value = listOf(refused("a", UnsentReason.CONFLICT))
            runCurrent()
            presenter.askToDiscardAll()
            runCurrent()

            presenter.discardAll()
            runCurrent()

            assertEquals(1, actions.discardedAll)
            assertFalse(presenter.state.value.confirmingDiscardAll)
        }

    @Test
    fun `answering no leaves the pile exactly as it was`() =
        runTest {
            val presenter = presenter()
            setAside.value = listOf(refused("a", UnsentReason.CONFLICT))
            runCurrent()
            presenter.askToDiscardAll()
            runCurrent()

            presenter.cancelDiscardAll()
            runCurrent()

            assertFalse(presenter.state.value.confirmingDiscardAll)
            assertEquals(0, actions.discardedAll)
            assertEquals(listOf("a"), presenter.state.value.setAside.map { it.id })
        }

    @Test
    fun `a pile emptied elsewhere takes the question with it`() =
        runTest {
            val presenter = presenter()
            setAside.value = listOf(refused("a", UnsentReason.CONFLICT))
            runCurrent()
            presenter.askToDiscardAll()
            runCurrent()

            // Another device's catch-up, or the sign-out wipe: the dialog would
            // otherwise be asking about a pile that is not there any more.
            setAside.value = emptyList()
            runCurrent()

            assertFalse(presenter.state.value.confirmingDiscardAll)
        }

    @Test
    fun `a device holding nothing back says so`() =
        runTest {
            val presenter = presenter()
            runCurrent()

            assertTrue(presenter.state.value.isEmpty)
        }

    private fun refused(
        id: String,
        reason: UnsentReason,
    ): UnsentChange =
        UnsentChange(
            id = id,
            kind = OutboxOpKind.TASK_COMPLETE,
            target = "Renew the domain",
            at = 1_000,
            reason = reason,
        )

    private fun queued(id: String): UnsentChange =
        UnsentChange(id = id, kind = OutboxOpKind.TASK_CREATE, target = "Buy milk", at = 2_000)
}

/** An actions port that records what it was asked for instead of touching a queue. */
private class RecordingUnsentActions : UnsentChangesActions {
    val discarded = mutableListOf<String>()
    var discardedAll = 0

    override suspend fun discard(id: String): Boolean {
        discarded += id
        return true
    }

    override suspend fun discardAll(): Int {
        discardedAll++
        return 1
    }
}
