package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.sync.write.TaskDestination

/**
 * What a change to a whole selection actually did.
 *
 * [leftBlocked] is separate from [changed] rather than folded into it because
 * the two need different sentences: a user who ticked off ten tasks and got
 * eight wants to know the other two are waiting on something, not that the
 * action half failed.
 *
 * [leftLocked] counts the ones the change was never the user's to make — a
 * project standing in the daily plan decides the priority of its work — which is
 * a different sentence again: there is nothing to wait for and nothing to retry.
 */
data class BulkChange(
    val changed: Int,
    val leftBlocked: Int = 0,
    val leftLocked: Int = 0,
)

/** Which change a selection was put through, so the screen can name it afterwards. */
enum class BulkAction {
    COMPLETED,
    MOVED,
    PRIORITISED,
    PLANNED,
    PARKED,
    DELETED,
    GROUPED,
}

/**
 * The writes a picked set of tasks can be put through.
 *
 * A port of its own rather than more methods on the single-task one: the actions
 * a list offers a selection are a different set from the ones it offers a row,
 * and only some screens offer them at all. A screen that does not simply has
 * none of this wired in.
 *
 * Three of these turn into a single request each, because the API answers a
 * whole selection at once for completing, moving and re-prioritising. Planning
 * and deleting have no such endpoint, so they are honestly one request per task;
 * that is a property of the API rather than a choice made here, and it is why
 * this port hands back what changed instead of a request count.
 */
interface BulkTaskActions {
    /**
     * Ticks off everything in the selection that can be ticked off.
     *
     * Anything an unfinished task still stands in front of is left alone and
     * counted separately, which is what the server does with the same list. The
     * device applies the rule itself so the count on screen is the real one
     * rather than an optimistic guess corrected a sync later.
     */
    suspend fun complete(taskLocalIds: List<Long>): BulkChange

    suspend fun move(
        taskLocalIds: List<Long>,
        destination: TaskDestination,
    ): BulkChange

    /**
     * Gives the selection one priority, leaving out whatever is not free to take
     * it.
     *
     * A task in a project that stands in the daily plan has its priority decided
     * by that project, and the server refuses the edit. It is dropped from the
     * batch here so the rest of the selection still goes through in one request.
     */
    suspend fun prioritise(
        taskLocalIds: List<Long>,
        priority: Priority,
    ): BulkChange

    suspend fun plan(
        taskLocalIds: List<Long>,
        state: PlanState,
    ): BulkChange

    suspend fun delete(taskLocalIds: List<Long>): BulkChange

    /**
     * Gathers the selection under a task created for the purpose.
     *
     * The children take the new parent's place, labels and priority — that is
     * what grouping means — so [destination] has to be somewhere structure can
     * live: a context, a project or one of its columns, never the inbox.
     */
    suspend fun group(
        title: String,
        childTaskLocalIds: List<Long>,
        destination: TaskDestination,
    ): BulkChange
}
