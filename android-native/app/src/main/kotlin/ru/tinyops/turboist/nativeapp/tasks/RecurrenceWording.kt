package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.sync.write.RecurrenceAdvancer

// Reading a repeat rule well enough to say it out loud, and writing the handful
// of rules a user picks from a list.
//
// A repeat rule is stored in the calendar notation the server and every other
// client speak, which is precise and unreadable. Showing it raw makes the field
// useless to anyone who does not already know the notation, and re-deriving the
// wording inside a composable would put a parser somewhere it cannot be tested.
// So the reading happens here and returns a shape; the screen turns the shape
// into the sentence, because only the screen has the translations.
//
// Nothing here decides whether a rule is *valid* — that answer belongs to the
// same calculator the completion path uses, and asking it twice would let the
// preview accept a rule the write path then ignores.

/** The repeat rules the editor offers under a name of their own. */
enum class RecurrencePreset(
    val rule: String,
) {
    DAILY("FREQ=DAILY"),
    WEEKDAYS("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"),
    WEEKLY("FREQ=WEEKLY"),
    MONTHLY("FREQ=MONTHLY"),
    YEARLY("FREQ=YEARLY"),
    ;

    companion object {
        /**
         * The preset [rule] is, if it is one of them.
         *
         * Compared on what the rule *says* rather than on how it is written, so
         * the same instruction spelled in another order, another case or by
         * another client is still recognised as the choice the user made instead
         * of falling through to the hand-written field.
         */
        fun of(rule: String?): RecurrencePreset? {
            val wording = recurrenceWording(rule)
            if (wording == RecurrenceWording.Never || wording == RecurrenceWording.Unreadable) return null
            if (wording is RecurrenceWording.AsWritten) return null
            return entries.firstOrNull { recurrenceWording(it.rule) == wording }
        }
    }
}

/** The seven day codes a weekly rule names its days with, in week order. */
enum class RecurrenceWeekday(
    val code: String,
) {
    MONDAY("MO"),
    TUESDAY("TU"),
    WEDNESDAY("WE"),
    THURSDAY("TH"),
    FRIDAY("FR"),
    SATURDAY("SA"),
    SUNDAY("SU"),
}

/**
 * What a repeat rule says, in the shapes the screen has words for.
 *
 * [AsWritten] is not a failure: the notation can express far more than a task
 * manager offers to write, and a rule the calculator understands is honoured
 * whether or not there is a sentence for it. [Unreadable] is the failure, and
 * the editor refuses to save one.
 */
sealed interface RecurrenceWording {
    /** The task does not repeat. */
    data object Never : RecurrenceWording

    data object EveryDay : RecurrenceWording

    data class EveryNumberOfDays(
        val days: Int,
    ) : RecurrenceWording

    /** Every week, on whichever day the task already falls on. */
    data object EveryWeek : RecurrenceWording

    /** Monday to Friday. */
    data object EveryWeekday : RecurrenceWording

    data class OnDaysOfTheWeek(
        val days: List<RecurrenceWeekday>,
    ) : RecurrenceWording

    /** Every month, on whichever day the task already falls on. */
    data object EveryMonth : RecurrenceWording

    data class OnDayOfTheMonth(
        val day: Int,
    ) : RecurrenceWording

    data object EveryYear : RecurrenceWording

    /** Understood, but with no sentence of its own: shown as the user wrote it. */
    data class AsWritten(
        val rule: String,
    ) : RecurrenceWording

    /** Not a repeat rule at all. */
    data object Unreadable : RecurrenceWording
}

/** How [rule] reads. */
@Suppress("ReturnCount")
fun recurrenceWording(rule: String?): RecurrenceWording {
    val text = rule?.trim().orEmpty()
    if (text.isEmpty()) return RecurrenceWording.Never
    if (!RecurrenceAdvancer.isReadable(text)) return RecurrenceWording.Unreadable
    val parts = recurrenceParts(text) ?: return RecurrenceWording.Unreadable
    // Each frequency has one refinement this screen has words for. Anything else
    // in the rule — a second refinement, an end date, a count — is real and is
    // honoured, but saying it in words here would mean saying it wrongly.
    val extras = parts.keys - "FREQ"
    val interval = parts["INTERVAL"]?.toIntOrNull() ?: 1
    return when (parts["FREQ"]) {
        "DAILY" ->
            when {
                extras.any { it != "INTERVAL" } -> RecurrenceWording.AsWritten(text)
                interval > 1 -> RecurrenceWording.EveryNumberOfDays(interval)
                else -> RecurrenceWording.EveryDay
            }
        "WEEKLY" ->
            if (extras.any { it != "BYDAY" }) {
                RecurrenceWording.AsWritten(text)
            } else {
                weeklyWording(parts["BYDAY"], text)
            }
        "MONTHLY" ->
            if (extras.any { it != "BYMONTHDAY" }) {
                RecurrenceWording.AsWritten(text)
            } else {
                monthlyWording(parts["BYMONTHDAY"], text)
            }
        "YEARLY" -> if (extras.isEmpty()) RecurrenceWording.EveryYear else RecurrenceWording.AsWritten(text)
        else -> RecurrenceWording.AsWritten(text)
    }
}

private fun weeklyWording(
    byDay: String?,
    text: String,
): RecurrenceWording {
    if (byDay == null) return RecurrenceWording.EveryWeek
    val named = byDay.split(",").map { it.trim() }
    val days = named.mapNotNull { code -> RecurrenceWeekday.entries.firstOrNull { it.code == code } }
    // An ordinal such as "3FR" is a different statement from a plain day and has
    // no sentence here; it is shown as written rather than described wrongly.
    if (days.size != named.size || days.isEmpty()) return RecurrenceWording.AsWritten(text)
    val inWeekOrder = RecurrenceWeekday.entries.filter { it in days }
    if (inWeekOrder == WORKING_WEEK) return RecurrenceWording.EveryWeekday
    return RecurrenceWording.OnDaysOfTheWeek(inWeekOrder)
}

private fun monthlyWording(
    byMonthDay: String?,
    text: String,
): RecurrenceWording {
    if (byMonthDay == null) return RecurrenceWording.EveryMonth
    val day = byMonthDay.toIntOrNull() ?: return RecurrenceWording.AsWritten(text)
    // A negative day counts back from the end of the month, which is a sentence
    // this screen does not write.
    if (day < 1) return RecurrenceWording.AsWritten(text)
    return RecurrenceWording.OnDayOfTheMonth(day)
}

/**
 * A rule broken into its named parts, or `null` when it is not written as named
 * parts at all.
 *
 * Case and order carry no meaning in the notation, so both are normalised away
 * — which is what lets a rule written by another client still be recognised as
 * one of the choices this screen offers.
 */
private fun recurrenceParts(rule: String?): Map<String, String>? {
    val text = rule?.trim().orEmpty()
    if (text.isEmpty()) return null
    val parts = mutableMapOf<String, String>()
    for (segment in text.split(";")) {
        val separator = segment.indexOf('=')
        if (separator <= 0) return null
        parts[segment.substring(0, separator).trim().uppercase()] = segment.substring(separator + 1).trim().uppercase()
    }
    return parts.takeIf { it.containsKey("FREQ") }
}

private val WORKING_WEEK: List<RecurrenceWeekday> =
    listOf(
        RecurrenceWeekday.MONDAY,
        RecurrenceWeekday.TUESDAY,
        RecurrenceWeekday.WEDNESDAY,
        RecurrenceWeekday.THURSDAY,
        RecurrenceWeekday.FRIDAY,
    )
