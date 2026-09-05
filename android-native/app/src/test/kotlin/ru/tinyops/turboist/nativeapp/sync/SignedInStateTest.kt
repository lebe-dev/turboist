package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.nativeapp.di.SyncModule
import ru.tinyops.turboist.nativeapp.session.SessionState
import ru.tinyops.turboist.nativeapp.session.SessionStateSource
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * What the triggers are told about the session, and when.
 *
 * The distinction that matters is between "there is no session" and "nobody has
 * looked yet". They lead to opposite actions — the first stops every background
 * job, the second must not touch them — and a process the system started to run
 * a sync job begins in the second state. Collapsing the two would make such a
 * process cancel, as its first act, the very job that started it: the catch-up
 * is dropped and the schedule is rebuilt from scratch, so a backgrounded device
 * loses a sync every time it is woken from a dead process.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SignedInStateTest {
    private class FakeSessions(initial: SessionState) : SessionStateSource {
        val mutable = MutableStateFlow(initial)
        override val state: StateFlow<SessionState> get() = mutable
    }

    private fun heard(
        source: FakeSessions,
        scope: kotlinx.coroutines.CoroutineScope,
    ): List<Boolean> {
        val heard = mutableListOf<Boolean>()
        scope.launch { SyncModule.signedInState(source).collect { heard += it } }
        return heard
    }

    @Test
    fun `an unread session says nothing rather than saying signed out`() =
        runTest {
            val sessions = FakeSessions(SessionState.Connecting)

            val heard = heard(sessions, backgroundScope)
            runCurrent()

            assertEquals(emptyList(), heard, "nothing is known yet, and acting on a guess cancels running work")
        }

    @Test
    fun `a session that resolves to nothing says signed out`() =
        runTest {
            val sessions = FakeSessions(SessionState.Connecting)

            val heard = heard(sessions, backgroundScope)
            sessions.mutable.value = SessionState.LoggedOut
            runCurrent()

            assertEquals(listOf(false), heard, "a device with no session has nothing to ask the server")
        }

    @Test
    fun `a session that resolves to credentials says signed in`() =
        runTest {
            val sessions = FakeSessions(SessionState.Connecting)

            val heard = heard(sessions, backgroundScope)
            sessions.mutable.value = SessionState.LoggedIn
            runCurrent()

            assertEquals(listOf(true), heard)
        }

    @Test
    fun `signing out after signing in is reported`() =
        runTest {
            val sessions = FakeSessions(SessionState.LoggedIn)

            val heard = heard(sessions, backgroundScope)
            runCurrent()
            sessions.mutable.value = SessionState.LoggedOut
            runCurrent()

            assertEquals(listOf(true, false), heard, "a logout still has to stop the background jobs")
        }
}
