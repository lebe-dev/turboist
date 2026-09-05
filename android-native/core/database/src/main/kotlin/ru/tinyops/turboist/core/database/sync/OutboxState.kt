package ru.tinyops.turboist.core.database.sync

/**
 * Where a queued write stands.
 *
 * The queue is drained strictly in order, one op at a time, because order is a
 * correctness property: a move that lands after the completion it preceded
 * produces a different result than the user asked for. So a stalled op stalls
 * the ops behind it rather than being skipped.
 */
enum class OutboxState(val stored: String) {
    /** Waiting its turn. The normal state of everything but the head of the queue. */
    PENDING("pending"),

    /** Sent, no answer yet. At most one op is ever in this state. */
    INFLIGHT("inflight"),

    /**
     * The last attempt failed in a way worth retrying — no network, or a server
     * error. The op keeps its place; only the backoff moved.
     */
    FAILED("failed"),
    ;

    companion object {
        private val byStored: Map<String, OutboxState> = entries.associateBy { it.stored }

        /**
         * The states that make the row an op points at *dirty*: a write is
         * queued against it, so an incoming change must not overwrite the
         * optimistic value the user is looking at. The op's own send, and the
         * pull that follows it, converge the row afterwards.
         */
        val BLOCKING: List<OutboxState> = listOf(PENDING, INFLIGHT, FAILED)

        /**
         * The state a stored value denotes.
         *
         * An unrecognised value becomes [FAILED] rather than [PENDING]: a value
         * this build cannot interpret must not be put on the wire, and [FAILED]
         * is the state that surfaces it to the user instead of sending it.
         */
        fun fromStored(value: String?): OutboxState = byStored[value] ?: FAILED
    }
}
