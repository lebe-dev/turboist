package ru.tinyops.turboist.core.sync.write

/**
 * What completing a repeating task does to it.
 *
 * A repeating task is not finished when it is ticked off — it moves to its next
 * occurrence, and the run just completed is recorded separately. Working out
 * when that next occurrence falls is a calendar problem, and getting it wrong on
 * screen is worse than not answering: a task that jumps to the wrong day and
 * then silently corrects itself reads as a bug in the user's plan, not in the
 * app.
 *
 * So the outcome is allowed to be [Unknown]. A build with no recurrence
 * calculator leaves the task exactly as it was and lets the server's answer bring
 * the truth, which is a small, honest delay rather than a confident wrong date.
 */
sealed interface RecurrenceOutcome {
    /** The device cannot say. The task is left untouched until the server answers. */
    data object Unknown : RecurrenceOutcome

    /** The series has no occurrence left, so this completion closes the task for good. */
    data object Ended : RecurrenceOutcome

    /** The task lives on, due again at [dueAt]. */
    data class Next(val dueAt: Long) : RecurrenceOutcome
}

/**
 * The seam where a recurrence calculator plugs into the write path.
 *
 * It is a seam rather than a call to a library so that the write path does not
 * depend on one, and so that the honest "I do not know" answer is the default
 * rather than an afterthought.
 */
fun interface RecurrenceAdvance {
    /**
     * When the task next falls due after being completed.
     *
     * @param rule the recurrence in the server's own notation.
     * @param currentDueAt the occurrence being completed, when the task had one.
     * @param completedAt the moment the user ticked it off.
     */
    fun after(
        rule: String,
        currentDueAt: Long?,
        completedAt: Long,
    ): RecurrenceOutcome

    companion object {
        /** Leaves every repeating task to the server. The default, and correct on its own. */
        val ServerDecides: RecurrenceAdvance = RecurrenceAdvance { _, _, _ -> RecurrenceOutcome.Unknown }
    }
}
