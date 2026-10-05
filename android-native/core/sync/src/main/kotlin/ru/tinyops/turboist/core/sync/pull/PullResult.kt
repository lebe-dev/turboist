package ru.tinyops.turboist.core.sync.pull

import ru.tinyops.turboist.core.network.ApiException

/**
 * How a catch-up ended.
 *
 * The three cases are the three different things a caller does next, which is
 * why they are separate types rather than a flag on one:
 *
 * - [Applied] — the replica is level with the server. Nothing to schedule.
 * - [Offline] — nothing reached the server. Try again when there is a network;
 *   the replica is untouched and every screen still works from it.
 * - [Refused] — the server answered and the answer was no. Retrying the same
 *   call changes nothing, so this needs credentials, an upgrade, or a person.
 *
 * A catch-up never throws for any of these. Losing contact with a server is an
 * ordinary state for an app that is expected to work without one, and an
 * ordinary state is a value.
 */
sealed interface PullResult {
    /**
     * The replica now holds the server's state as of [cursor].
     *
     * @property changes how many records were carried over. Zero is the common
     *   answer and means the replica was already current.
     * @property fromSnapshot true when this was a fresh copy of everything rather
     *   than a catch-up: a first launch, or a history the device could no longer
     *   resume against.
     */
    data class Applied(
        val epoch: Long,
        val cursor: Long,
        val changes: Int,
        val fromSnapshot: Boolean,
    ) : PullResult

    /** Nothing reached the server. The replica is exactly as it was. */
    data class Offline(val cause: ApiException.Network) : PullResult

    /** The server answered, and refused. */
    data class Refused(val cause: ApiException) : PullResult

    /** True when the replica moved to server truth. */
    val isApplied: Boolean
        get() = this is Applied
}
