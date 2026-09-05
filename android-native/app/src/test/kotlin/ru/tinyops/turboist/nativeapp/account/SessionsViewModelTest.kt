package ru.tinyops.turboist.nativeapp.account

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Before
import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The list of ways into the account, with the server replaced by a stand-in.
 *
 * Three rules are worth pinning here rather than leaving to the screen.
 *
 * A list that could not be fetched is *not* an account with nothing signed in.
 * The two look alike and mean opposite things, and showing the second when the
 * first is true is false reassurance about exactly the question the user opened
 * this screen to ask.
 *
 * A session leaves the list only once the server has agreed. Dropping a row on
 * the strength of a call that failed would tell someone a device is locked out
 * while it is still signed in.
 *
 * Nothing here happens on one tap. Revoking cannot be undone, and signing out
 * everywhere takes this device with it — including the work the server has not
 * accepted, which is why that question carries the number.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SessionsViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val control = FakeSessionControl()

    @Before
    fun useTestDispatcher() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun releaseDispatcher() {
        Dispatchers.resetMain()
    }

    @Test
    fun `the open sessions arrive and the screen stops waiting`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2))

            val model = SessionsViewModel(control)
            advanceUntilIdle()

            val state = model.state.value
            assertFalse(state.loading)
            assertFalse(state.unreachable)
            assertEquals(listOf(1L, 2L), state.sessions.map { it.id })
        }

    @Test
    fun `a list that could not be fetched is said to be missing, not empty`() =
        runTest(dispatcher) {
            control.listFailure = offline()

            val model = SessionsViewModel(control)
            advanceUntilIdle()

            assertTrue(model.state.value.unreachable, "an unfetchable list was reported as an empty one")
            assertEquals(AccountProblem.OFFLINE, model.state.value.problem)
            assertTrue(model.state.value.sessions.isEmpty())

            // And a retry that works clears it rather than leaving the warning up.
            control.listFailure = null
            control.stored += sessionOf(1, current = true)
            model.load()
            advanceUntilIdle()

            assertFalse(model.state.value.unreachable)
            assertEquals(listOf(1L), model.state.value.sessions.map { it.id })
        }

    @Test
    fun `revoking asks first and only then ends the session`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2))
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            val other = model.state.value.sessions.first { !it.isCurrent }
            model.ask(SessionsConfirmation.Revoke(other))
            advanceUntilIdle()

            // Nothing has happened yet: the question is up.
            assertTrue(control.revoked.isEmpty())
            assertEquals(SessionsConfirmation.Revoke(other), model.state.value.confirming)

            model.confirm()
            advanceUntilIdle()

            assertEquals(listOf(2L), control.revoked)
            assertEquals(listOf(1L), model.state.value.sessions.map { it.id })
            assertNull(model.state.value.confirming)
        }

    @Test
    fun `a question the user backs out of changes nothing`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2))
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            model.ask(SessionsConfirmation.Revoke(model.state.value.sessions.last()))
            model.cancelConfirmation()
            advanceUntilIdle()

            assertNull(model.state.value.confirming)
            assertTrue(control.revoked.isEmpty())
            assertEquals(listOf(1L, 2L), model.state.value.sessions.map { it.id })
        }

    @Test
    fun `a refused revocation leaves the session on the list it is still on`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2))
            control.revokeFailure = refusal()
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            model.ask(SessionsConfirmation.Revoke(model.state.value.sessions.last()))
            model.confirm()
            advanceUntilIdle()

            // Guessing at this list is the one thing the screen must not do: a
            // row removed here would claim a device is locked out when it is not.
            assertEquals(listOf(1L, 2L), model.state.value.sessions.map { it.id })
            assertEquals(AccountProblem.REFUSED, model.state.value.problem)
            assertFalse(model.state.value.busy)
        }

    @Test
    fun `signing out of the others leaves this device signed in`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2), sessionOf(3))
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            model.ask(SessionsConfirmation.SignOutOthers)
            model.confirm()
            advanceUntilIdle()

            assertEquals(1, control.othersSignedOut)
            assertEquals(listOf(1L), model.state.value.sessions.map { it.id })
            assertEquals(SessionsMessage.OTHERS_SIGNED_OUT, model.state.value.message)
            assertEquals(0, control.signedOutEverywhere)
        }

    @Test
    fun `signing out everywhere is asked with the count of work that goes with it`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2))
            control.unsent = 3
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            model.ask(SessionsConfirmation.SignOutEverywhere)
            advanceUntilIdle()

            // The number is read when the question goes up rather than when the
            // screen opened: the queue drains in the background, and a stale
            // number would frighten the user about work that has already gone out.
            assertEquals(3, model.state.value.unsentChangeCount)
            assertEquals(0, control.signedOutEverywhere)

            model.confirm()
            advanceUntilIdle()

            assertEquals(1, control.signedOutEverywhere)
        }

    @Test
    fun `a second action cannot start while the first is out`() =
        runTest(dispatcher) {
            control.stored += listOf(sessionOf(1, current = true), sessionOf(2), sessionOf(3))
            val model = SessionsViewModel(control)
            advanceUntilIdle()

            model.ask(SessionsConfirmation.Revoke(model.state.value.sessions.last()))
            model.confirm()
            // Nothing has been advanced, so the first call is still out.
            model.ask(SessionsConfirmation.SignOutOthers)

            assertNull(model.state.value.confirming, "a second question went up over a call still in flight")
            advanceUntilIdle()

            assertEquals(listOf(3L), control.revoked)
            assertEquals(0, control.othersSignedOut)
        }
}
