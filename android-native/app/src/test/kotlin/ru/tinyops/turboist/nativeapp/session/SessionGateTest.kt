package ru.tinyops.turboist.nativeapp.session

import kotlin.test.Test
import kotlin.test.assertEquals

class SessionGateTest {
    @Test
    fun `an unresolved session shows the splash, never the sign-in form`() {
        // Bouncing to sign-in before the stored token has been read would make
        // every cold start look like a logout.
        assertEquals(ShellGraph.Splash, graphFor(SessionState.Connecting))
    }

    @Test
    fun `no usable credentials leads to the authentication screens`() {
        assertEquals(ShellGraph.Auth, graphFor(SessionState.LoggedOut))
    }

    @Test
    fun `usable credentials lead to the app`() {
        assertEquals(ShellGraph.App, graphFor(SessionState.LoggedIn))
    }
}
