package ru.tinyops.turboist.nativeapp.auth

import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * The rule that decides what a launch does.
 *
 * The case worth being loud about is the third one: a phone with no signal must
 * not be treated as a phone whose credentials were refused. Getting that wrong
 * locks a user out of data already on their device, at the exact moment they
 * cannot sign in again because the server is the thing they cannot reach.
 */
class BootDecisionTest {
    @Test
    fun `a fresh install asks which server`() {
        assertEquals(
            BootDecision.Connect,
            bootDecision(serverConfigured = false, renew = RenewOutcome.NO_TOKEN, setup = SetupProbe.UNKNOWN),
        )
    }

    @Test
    fun `a renewed session opens the app`() {
        assertEquals(
            BootDecision.App,
            bootDecision(serverConfigured = true, renew = RenewOutcome.RENEWED, setup = SetupProbe.UNKNOWN),
        )
    }

    @Test
    fun `an unreachable server opens the app on the replica instead of signing the user out`() {
        assertEquals(
            BootDecision.OfflineApp,
            bootDecision(serverConfigured = true, renew = RenewOutcome.UNREACHABLE, setup = SetupProbe.UNKNOWN),
        )
    }

    @Test
    fun `a refused token ends the session`() {
        assertEquals(
            BootDecision.SignIn,
            bootDecision(serverConfigured = true, renew = RenewOutcome.REJECTED, setup = SetupProbe.UNKNOWN),
        )
    }

    @Test
    fun `a server with no account yet asks for one to be created`() {
        assertEquals(
            BootDecision.Setup,
            bootDecision(serverConfigured = true, renew = RenewOutcome.NO_TOKEN, setup = SetupProbe.REQUIRED),
        )
    }

    @Test
    fun `a server with an account asks for a sign-in`() {
        assertEquals(
            BootDecision.SignIn,
            bootDecision(serverConfigured = true, renew = RenewOutcome.NO_TOKEN, setup = SetupProbe.DONE),
        )
    }

    @Test
    fun `an unanswered setup question still offers a sign-in`() {
        // Guessing "create an account" against a server that has one would show a
        // screen whose only outcome is a refusal. The sign-in attempt, in contrast,
        // comes back saying setup is needed and the flow moves on by itself.
        assertEquals(
            BootDecision.SignIn,
            bootDecision(serverConfigured = true, renew = RenewOutcome.NO_TOKEN, setup = SetupProbe.UNKNOWN),
        )
    }

    @Test
    fun `no server means no screen but the server screen, whatever else is true`() {
        // A stored token without a stored address cannot be used for anything, so
        // the address question always comes first.
        for (outcome in RenewOutcome.entries) {
            for (setup in SetupProbe.entries) {
                assertEquals(
                    BootDecision.Connect,
                    bootDecision(serverConfigured = false, renew = outcome, setup = setup),
                )
            }
        }
    }
}
