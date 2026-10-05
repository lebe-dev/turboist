package ru.tinyops.turboist.nativeapp.unsent

import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.write.OutboxOpKind

/**
 * Why a change will never reach the server.
 *
 * The server's own code is what this is derived from rather than the HTTP
 * status, because several unrelated refusals share a status and only the code
 * tells them apart. Codes that mean the same thing to a person are folded
 * together: what a screen has to convey is what the user can do next, and "the
 * details were not accepted" is one answer whether the server called it invalid
 * or called the recurrence invalid.
 */
enum class UnsentReason {
    /** Work that stands in the way of the task was still open. */
    BLOCKED,

    /** The row the change was for is not on the server any more. */
    TARGET_GONE,

    /** A cap — pinned items, the plan for the week — was already full. */
    LIMIT_REACHED,

    /** The server would not put the item where the change was sending it. */
    PLACEMENT_REFUSED,

    /** The slot of the daily plan already holds as much as it takes. */
    TROIKI_SLOT_FULL,

    /** The server did not accept the details of the change. */
    INVALID,

    /** Something else had already changed underneath it. */
    CONFLICT,

    /** The session was not allowed to make this change. */
    NOT_ALLOWED,

    /** The change could never be turned into a request, so it was never sent. */
    UNSENDABLE,

    /** The server broke on it, repeatedly. */
    SERVER_ERROR,

    /** The server refused, and said nothing this build understands. */
    UNKNOWN,
    ;

    companion object {
        /**
         * The reason a stored error code denotes.
         *
         * A code this build has never heard of becomes [UNKNOWN] rather than
         * failing: the pile is read long after the refusal, possibly by a build
         * older than the server that produced it, and a list that cannot be
         * rendered is worse than one row that reads vaguely.
         */
        fun of(errorCode: String): UnsentReason =
            when (errorCode) {
                ApiErrorCodes.TASK_BLOCKED -> BLOCKED
                ApiErrorCodes.TARGET_GONE, ApiErrorCodes.NOT_FOUND -> TARGET_GONE
                ApiErrorCodes.LIMIT_EXCEEDED -> LIMIT_REACHED
                ApiErrorCodes.FORBIDDEN_PLACEMENT -> PLACEMENT_REFUSED
                ApiErrorCodes.TROIKI_SLOT_FULL -> TROIKI_SLOT_FULL
                ApiErrorCodes.VALIDATION_FAILED, ApiErrorCodes.RECURRENCE_INVALID -> INVALID
                ApiErrorCodes.CONFLICT -> CONFLICT
                ApiErrorCodes.FORBIDDEN -> NOT_ALLOWED
                ApiErrorCodes.WRITE_UNSENDABLE -> UNSENDABLE
                ApiErrorCodes.INTERNAL_ERROR -> SERVER_ERROR
                else -> UNKNOWN
            }
    }
}

/**
 * One change the server has not been told about, in the terms a person reads it
 * by rather than the terms it is sent in.
 *
 * @property id the queue's own id for the change, which is what a discard names.
 * @property kind which write it is, or `null` when the stored name comes from a
 *   build this one does not know — the row is still shown, because a change the
 *   app cannot describe is exactly the kind a user must not lose silently.
 * @property target the title of the row the change was for, resolved from the
 *   replica. `null` when the row is gone, which is itself common here: the most
 *   frequent refusal is a change aimed at something deleted elsewhere.
 * @property at when this happened — queued, for a change still waiting; refused,
 *   for one the server turned down.
 * @property reason `null` while the change is still expected to go out.
 * @property blockers titles of the still-open work that stood in the way, when
 *   that is what the refusal was about. The refusal's own wording is the same
 *   sentence every time, and these are the only part of it that says where to
 *   go and look.
 */
data class UnsentChange(
    val id: String,
    val kind: OutboxOpKind?,
    val target: String?,
    val at: Long,
    val reason: UnsentReason? = null,
    val blockers: List<String> = emptyList(),
)
