package ru.tinyops.turboist.nativeapp.sync

import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * What the shell decides to say, given what it knows.
 *
 * The rule this pins down is that an app which is keeping up says nothing. A
 * strip that is always there is a strip nobody reads, so the only states that
 * earn one are the states where what the user believes about the screen would
 * otherwise be wrong — or where work of theirs is sitting on the device.
 */
class SyncStatusTest {
    @Test
    fun `an app that is keeping up says nothing`() {
        assertEquals(SyncNotice.None, SyncStatus(outcome = SyncOutcome.SYNCED, lastSyncAt = 1_000).notice())
    }

    @Test
    fun `a cycle that is running is not itself news`() {
        // The hairline says a cycle is running. A strip would say it too, on
        // every automatic cycle, which is most of the time the app is open.
        val status = SyncStatus(syncing = true, outcome = SyncOutcome.SYNCED, lastSyncAt = 1_000)

        assertEquals(SyncNotice.None, status.notice())
    }

    @Test
    fun `a server that cannot be reached is said, with the age of what is on screen`() {
        val status = SyncStatus(outcome = SyncOutcome.UNREACHABLE, lastSyncAt = 1_700, waiting = 2)

        assertEquals(SyncNotice.Unreachable(lastSyncAt = 1_700, waiting = 2), status.notice())
    }

    @Test
    fun `a device that has never synced dates nothing`() {
        val status = SyncStatus(outcome = SyncOutcome.UNREACHABLE)

        assertEquals(SyncNotice.Unreachable(lastSyncAt = null, waiting = 0), status.notice())
    }

    @Test
    fun `a turn the server refused is its own state, because trying again is a real question`() {
        val status = SyncStatus(outcome = SyncOutcome.REFUSED, lastSyncAt = 900, waiting = 1)

        assertEquals(SyncNotice.Failed(waiting = 1), status.notice())
    }

    @Test
    fun `work that has not gone out is said even while everything else is fine`() {
        val status = SyncStatus(outcome = SyncOutcome.SYNCED, lastSyncAt = 5, waiting = 3)

        assertEquals(SyncNotice.Waiting(3), status.notice())
    }

    @Test
    fun `a session that could not be checked reads as a server that could not be reached`() {
        // It means the same thing to the person looking at the screen — nothing
        // has come from the server this run — and saying it twice in two strips
        // would be two ways of saying one thing.
        val status = SyncStatus(outcome = SyncOutcome.UNKNOWN, lastSyncAt = 42, waiting = 1)

        assertEquals(
            SyncNotice.Unreachable(lastSyncAt = 42, waiting = 1),
            status.notice(sessionUnverified = true),
        )
    }

    @Test
    fun `a cycle that reached the server outranks a session nobody has checked`() {
        // The session gate is a launch-time guess; a finished cycle is evidence.
        val status = SyncStatus(outcome = SyncOutcome.REFUSED)

        assertEquals(SyncNotice.Failed(waiting = 0), status.notice(sessionUnverified = true))
    }

    @Test
    fun `a queue that cannot move is part of the same news, not a second piece of it`() {
        val status = SyncStatus(outcome = SyncOutcome.UNREACHABLE, waiting = 4)

        assertEquals(SyncNotice.Unreachable(lastSyncAt = null, waiting = 4), status.notice())
    }

    @Test
    fun `a fresh install with nothing queued and nothing tried says nothing`() {
        assertEquals(SyncNotice.None, SyncStatus().notice())
    }
}
