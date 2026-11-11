package ru.tinyops.turboist.core.model

/**
 * The Today focus lock, as a rule over plain facts.
 *
 * Turned on from the Today screen, the lock runs until the next midnight and
 * cannot be lifted early. While it runs, a task may only be taken into Today —
 * given a date of today or earlier while open — in a project that already has
 * open work there. The inbox, and a task with no project at all, never qualify.
 * Adding work for another day, or with no date, is always allowed.
 *
 * The set of allowed projects is read live, never remembered: once the last
 * task of a project for today is done, that project is closed for the rest of
 * the day. This is the server's rule (`service.TodayLockService`); the device
 * repeats it so a refused write says no while the user is still looking at it
 * instead of coming back undone from the queue.
 */
object TodayLock {
    /** Whether a lock ending at [until] still runs at [now]. Both are epoch milliseconds. */
    fun isActive(
        until: Long?,
        now: Long,
    ): Boolean = until != null && now < until

    /**
     * Where a task stands as far as the lock cares.
     *
     * @property projectLocalId `null` for the inbox or a bare context.
     */
    data class Placement(
        val projectLocalId: Long?,
        val dueAt: Long?,
        val open: Boolean,
    )

    /**
     * Whether a write that leaves a task at [after] is refused.
     *
     * [before] is the task as it was, `null` for a new one: a task that already
     * stood on Today in the same project takes nothing new into the day, so
     * editing it or moving it to another hour passes. [todayProjects] is the set
     * of projects with open work on Today *other than this task*; it is only
     * consulted when the answer depends on it, so a caller may compute it lazily.
     */
    fun refuses(
        until: Long?,
        now: Long,
        before: Placement?,
        after: Placement,
        todayProjects: () -> Set<Long>,
    ): Boolean {
        if (!isActive(until, now) || until == null) return false
        if (!onToday(after, until)) return false
        if (before != null && onToday(before, until) && before.projectLocalId == after.projectLocalId) return false
        val project = after.projectLocalId ?: return true
        return project !in todayProjects()
    }

    private fun onToday(
        placement: Placement,
        until: Long,
    ): Boolean = placement.open && placement.dueAt != null && placement.dueAt < until
}
