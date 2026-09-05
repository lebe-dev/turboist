package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.network.ApiErrorCodes

/**
 * A write the device refused to make.
 *
 * The server owns the rules; the device repeats only the few of them whose
 * violation would otherwise be invisible until the queue drained. Refusing here
 * is not defensive programming, it is honesty: an action that queues, sits in
 * the queue for a day and then comes back rejected is worse than an action that
 * says no while the user is still looking at the thing they tried to change.
 *
 * [code] is the server's own code for the same refusal, so a message written
 * once serves both cases and the user never learns that two different layers
 * said no.
 */
sealed class WriteRefused(
    val code: String,
    message: String,
) : Exception(message) {
    /**
     * The task is held up by another one that is still open.
     *
     * The blockers are inherited down the subtask tree: a subtask of a blocked
     * task is blocked too, because finishing it would claim progress on work
     * that cannot start. A blocker inside the task's own subtree does not count —
     * that is the work itself, not something outside waiting on it.
     */
    class TaskBlocked(val blockerLocalIds: List<Long>) :
        WriteRefused(
            ApiErrorCodes.TASK_BLOCKED,
            "the task is blocked by ${blockerLocalIds.size} open task(s)",
        )

    /** The pinned shelf is full. Its size is the user's own setting, read fresh on every pin. */
    class PinLimitReached(val limit: Int) :
        WriteRefused(ApiErrorCodes.LIMIT_EXCEEDED, "no more than $limit may be pinned")

    /** The daily slot already holds as many projects as it takes. */
    class TroikiSlotFull(val category: TroikiCategory, val capacity: Int) :
        WriteRefused(
            ApiErrorCodes.TROIKI_SLOT_FULL,
            "the ${category.wire} slot already holds $capacity project(s)",
        )

    /**
     * The row the write names is not in the replica.
     *
     * This is a programming error rather than a user-facing one — a screen
     * acting on something it read from the replica cannot have lost it — so it
     * carries the row it looked for.
     */
    class RowMissing(val what: String, val localId: Long) :
        WriteRefused(ApiErrorCodes.NOT_FOUND, "no $what with local id $localId")

    /**
     * The row cannot go where the write puts it. Placement is exclusive and some
     * combinations are simply not things — a subtask of a task that lives in the
     * inbox, for one.
     */
    class Placement(reason: String) : WriteRefused(ApiErrorCodes.FORBIDDEN_PLACEMENT, reason)

    /**
     * The two tasks are already linked that way.
     *
     * The store keeps one row per pair and kind, so a second attempt is not a
     * second link — it is the same one, asked for twice. A symmetric link counts
     * as the same from either end.
     */
    class RelationExists(val relationLocalId: Long) :
        WriteRefused(ApiErrorCodes.CONFLICT, "the two tasks are already linked that way")

    /**
     * The link would leave a group of tasks waiting on each other forever.
     *
     * Every task in such a loop is held up by the next one, so none of them can
     * ever be finished and nothing inside the product can undo it but deleting a
     * link. It is refused at the moment it is made, which is the only moment
     * there is still something to refuse.
     */
    class RelationCycle(val blockerLocalId: Long, val blockedLocalId: Long) :
        WriteRefused(
            ApiErrorCodes.VALIDATION_FAILED,
            "the link would leave the two tasks waiting on each other",
        )

    /** The write is malformed in a way the server would reject anyway. */
    class Invalid(reason: String) : WriteRefused(ApiErrorCodes.VALIDATION_FAILED, reason)
}
