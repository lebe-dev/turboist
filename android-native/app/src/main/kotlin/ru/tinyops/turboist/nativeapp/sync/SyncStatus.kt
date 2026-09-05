package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.flow.StateFlow

/**
 * How the last finished cycle ended.
 *
 * Deliberately coarser than the engine's own answer: a screen has three things
 * to say and no more — the two sides agree, the server could not be reached, or
 * it was reached and the turn did not complete. Which request failed and with
 * what is a matter for the log, not for a strip above a task list.
 */
enum class SyncOutcome {
    /** No cycle has finished since the app started. */
    UNKNOWN,

    /** The replica and the server agreed, as of that cycle. */
    SYNCED,

    /** Nothing reached the server. Everything on screen is the device's own copy. */
    UNREACHABLE,

    /** The server answered and the turn did not complete. */
    REFUSED,
}

/**
 * What the shell knows about how current the data on screen is.
 *
 * [syncing] is kept apart from [outcome] on purpose. A cycle that starts does
 * not undo what the previous one concluded, so an app that has been out of
 * coverage keeps saying so while it tries again, instead of flickering between
 * "no connection" and nothing at all once per attempt.
 *
 * @property lastSyncAt when the replica was last brought level with the server,
 *   in epoch milliseconds, or `null` when it never has been.
 * @property waiting how many changes are queued and still expected to go out.
 * @property setAside how many the server refused, which only a person can clear.
 */
data class SyncStatus(
    val syncing: Boolean = false,
    val outcome: SyncOutcome = SyncOutcome.UNKNOWN,
    val lastSyncAt: Long? = null,
    val waiting: Int = 0,
    val setAside: Int = 0,
)

/**
 * What the shell has to tell the user, if anything.
 *
 * Most of the time the answer is [None]: an app that is keeping up says nothing,
 * because a permanent strip claiming everything is fine is a permanent strip
 * that is ignored. The three cases that do earn one are the three the user can
 * act on — the server cannot be reached, it refused a turn, or work of theirs is
 * still queued.
 */
sealed interface SyncNotice {
    /** Nothing to say. */
    data object None : SyncNotice

    /** Reachable, and [count] changes are still on their way out. */
    data class Waiting(val count: Int) : SyncNotice

    /**
     * The server could not be reached, so the screen is the device's own copy.
     *
     * @property lastSyncAt when that copy was last level with the server, or
     *   `null` when it never has been — a fresh install with no signal.
     */
    data class Unreachable(val lastSyncAt: Long?, val waiting: Int) : SyncNotice

    /** The server was reached and the turn did not complete. */
    data class Failed(val waiting: Int) : SyncNotice
}

/**
 * The notice a status calls for.
 *
 * [sessionUnverified] is true while the app is rendering a stored session it has
 * not been able to check with the server. It reads as unreachable because it
 * means the same thing to the person looking at the screen — nothing has come
 * from the server this run — and saying it twice in two different strips would
 * be two ways of saying one thing.
 *
 * The order is the order of severity: a queue that cannot move is a detail of
 * not being able to reach the server, not a separate piece of news.
 */
fun SyncStatus.notice(sessionUnverified: Boolean = false): SyncNotice =
    when {
        outcome == SyncOutcome.UNREACHABLE -> SyncNotice.Unreachable(lastSyncAt, waiting)
        outcome == SyncOutcome.REFUSED -> SyncNotice.Failed(waiting)
        sessionUnverified -> SyncNotice.Unreachable(lastSyncAt, waiting)
        waiting > 0 -> SyncNotice.Waiting(waiting)
        else -> SyncNotice.None
    }

/**
 * Where the shell reads the sync status from.
 *
 * A seam like the session one: the shell asks an interface, so a screen can be
 * composed against a status that was written out by hand rather than produced by
 * an engine, a database and a radio.
 */
interface SyncStatusSource {
    val status: StateFlow<SyncStatus>
}
