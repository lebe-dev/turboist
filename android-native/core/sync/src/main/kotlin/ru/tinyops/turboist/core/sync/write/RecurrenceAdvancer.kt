package ru.tinyops.turboist.core.sync.write

import android.util.Log
import org.dmfs.rfc5545.DateTime
import org.dmfs.rfc5545.recur.InvalidRecurrenceRuleException
import org.dmfs.rfc5545.recur.RecurrenceRule
import java.time.ZoneId
import java.util.TimeZone

/**
 * Works out when a repeating task next falls due, the way the server would.
 *
 * The server owns this arithmetic, and the device only repeats it so that a task
 * ticked off with no connection can show its next run at once instead of an hour
 * later. That makes agreement the whole point: a date computed here and quietly
 * replaced by a different one after the next catch-up reads as a bug in the
 * user's plan rather than in the app. The two implementations are therefore held
 * to one shared list of worked examples, and neither side changes its answer
 * without the other.
 *
 * Three rules make up the whole of it.
 *
 * 1. **The anchor is where the task sits, not when it was ticked off** — unless
 *    it is late, in which case it is the tick. A task finished three weeks after
 *    it was due resumes from today rather than marching through every run it
 *    missed.
 * 2. **The anchor is also where the series starts.** A task carries a rule, not
 *    a start date, so the rule is read relative to the occurrence in hand. That
 *    is what makes "every third day" mean three days from this one.
 * 3. **The zone is part of the question.** A rule repeats a wall clock, so a
 *    daily task at nine stays at nine across a daylight-saving change even
 *    though the elapsed hours are not twenty-four; and a task due on a day
 *    rather than at a time stays on local midnight for the same reason.
 *
 * A rule this build cannot read is answered with [RecurrenceOutcome.Unknown]
 * rather than with a guess: leaving the task alone until the server answers is a
 * short honest delay, and a confident wrong date is not.
 */
class RecurrenceAdvancer(
    private val zone: ZoneId = ZoneId.systemDefault(),
) : RecurrenceAdvance {
    override fun after(
        rule: String,
        currentDueAt: Long?,
        completedAt: Long,
    ): RecurrenceOutcome {
        val parsed =
            readRule(rule) ?: run {
                Log.w(RECURRENCE_LOG_TAG, "A task repeats by a rule this app cannot read; leaving it to the server")
                return RecurrenceOutcome.Unknown
            }
        val anchor = anchorFor(currentDueAt, completedAt)
        val next = firstAfter(parsed, anchor, zone) ?: return RecurrenceOutcome.Ended
        return RecurrenceOutcome.Next(next)
    }

    companion object {
        /** The log tag the recurrence advance shares with the rest of the sync engine. */
        private const val RECURRENCE_LOG_TAG = "TurboistSync"

        /**
         * How many instances are read before the search gives up.
         *
         * Two are enough: the series starts at the anchor and never goes
         * backwards, so the answer is the first or the second. The bound exists
         * because a rule is user input, and a rule that somehow stood still would
         * otherwise spin inside a database transaction.
         */
        private const val INSTANCE_READ_LIMIT: Int = 8

        /**
         * The occurrence the next one is measured from: the one the task still
         * sits on when that is ahead, and otherwise the moment it was ticked off.
         */
        fun anchorFor(
            currentDueAt: Long?,
            completedAt: Long,
        ): Long = if (currentDueAt != null && currentDueAt > completedAt) currentDueAt else completedAt

        /** Whether this build can read [rule] and compute with it. */
        fun isReadable(rule: String): Boolean = readRule(rule) != null

        /**
         * Reads a rule, or answers that it is not one.
         *
         * Silent on purpose. This is the same question the repeat editor asks of
         * every keystroke, and a log line per keystroke would bury the sync log in
         * the half-typed rules a user passes through on the way to a good one. The
         * one place a refusal matters — a task that could not be moved on — says
         * so where it happens.
         */
        private fun readRule(rule: String): RecurrenceRule? {
            val text = rule.trim()
            if (text.isEmpty()) return null
            return try {
                RecurrenceRule(text, RecurrenceRule.RfcMode.RFC5545_LAX)
            } catch (malformed: InvalidRecurrenceRuleException) {
                null
            } catch (malformed: IllegalArgumentException) {
                // Parts that parse but cannot be combined are reported this way
                // rather than as a rule exception, and mean the same thing here.
                null
            }
        }

        /**
         * The first instance strictly after [anchor], or `null` when the series
         * has none left.
         *
         * Strictly after, because the anchor is the run being completed. The
         * comparison is on instants rather than on calendar fields, since a rule
         * is expanded against a wall clock while the replica stores moments.
         */
        private fun firstAfter(
            rule: RecurrenceRule,
            anchor: Long,
            zone: ZoneId,
        ): Long? {
            val instances = rule.iterator(DateTime(TimeZone.getTimeZone(zone), anchor))
            var read = 0
            while (instances.hasNext() && read < INSTANCE_READ_LIMIT) {
                read++
                val instance = instances.nextMillis()
                if (instance > anchor) return instance
            }
            return null
        }
    }
}
