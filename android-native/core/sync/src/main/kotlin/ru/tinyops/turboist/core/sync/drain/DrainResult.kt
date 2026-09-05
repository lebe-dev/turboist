package ru.tinyops.turboist.core.sync.drain

import ru.tinyops.turboist.core.network.ApiException

/**
 * How a pass over the queue of unsent writes ended.
 *
 * The three cases are the three different things that happen next, which is why
 * they are separate types rather than a flag on one:
 *
 * - [Drained] — nothing is waiting any more. The replica can be caught up.
 * - [Stalled] — the write at the head could not be sent and nothing behind it may
 *   overtake it, so the queue waits. Try again later; [retryAfterMillis] says how
 *   much later at the earliest.
 * - [NeedsCredentials] — the server refused the write on who is asking rather
 *   than on what was asked. Retrying changes nothing until the user signs in
 *   again, and nothing is thrown away in the meantime.
 *
 * A drain never throws for any of these. Being unable to reach a server is an
 * ordinary state for an app expected to work without one, and an ordinary state
 * is a value.
 */
sealed interface DrainResult {
    /** How many writes reached the server in this pass. */
    val sent: Int

    /** How many were refused for good and set aside for the user to look at. */
    val quarantined: Int

    /** The queue is empty. */
    data class Drained(
        override val sent: Int,
        override val quarantined: Int,
    ) : DrainResult

    /**
     * The head of the queue could not be sent, and the queue stops there.
     *
     * Stopping rather than skipping is the point: order is a correctness
     * property, and a move applied after the completion it preceded produces a
     * different result than the user asked for.
     *
     * @property remaining how many writes are still waiting, the head included.
     * @property cause what the attempt failed with — no network, a server that
     *   broke, or a repeat of a key the server is still working on.
     * @property retryAfterMillis the earliest another attempt is worth making.
     */
    data class Stalled(
        override val sent: Int,
        override val quarantined: Int,
        val remaining: Int,
        val cause: ApiException,
        val retryAfterMillis: Long,
    ) : DrainResult

    /** The server will not accept writes from this session until the user signs in again. */
    data class NeedsCredentials(
        override val sent: Int,
        override val quarantined: Int,
        val remaining: Int,
        val cause: ApiException,
    ) : DrainResult

    /** True when the queue is empty — the only state from which the replica is certainly current. */
    val isDrained: Boolean
        get() = this is Drained
}
