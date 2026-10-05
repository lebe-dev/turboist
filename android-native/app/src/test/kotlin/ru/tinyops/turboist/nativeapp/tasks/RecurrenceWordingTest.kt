package ru.tinyops.turboist.nativeapp.tasks

import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * Reading a repeat rule well enough to say it out loud.
 *
 * The notation is exact and unreadable, and a field showing it raw is useless to
 * anyone who does not already know it. What these cases hold to account is the
 * line between the three answers: a rule with a sentence, a rule that is real
 * but has no sentence here, and a rule that is not a rule. Only the third may be
 * refused, because the notation can express far more than this screen offers to
 * write, and a task set up on the web must stay legible and editable on a phone.
 */
class RecurrenceWordingTest {
    @Test
    fun `no rule at all reads as not repeating`() {
        assertEquals(RecurrenceWording.Never, recurrenceWording(null))
        assertEquals(RecurrenceWording.Never, recurrenceWording(" "))
    }

    @Test
    fun `the named choices read as their own names`() {
        assertEquals(RecurrenceWording.EveryDay, recurrenceWording("FREQ=DAILY"))
        assertEquals(RecurrenceWording.EveryWeekday, recurrenceWording("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"))
        assertEquals(RecurrenceWording.EveryWeek, recurrenceWording("FREQ=WEEKLY"))
        assertEquals(RecurrenceWording.EveryMonth, recurrenceWording("FREQ=MONTHLY"))
        assertEquals(RecurrenceWording.EveryYear, recurrenceWording("FREQ=YEARLY"))
    }

    @Test
    fun `an interval and a chosen set of days read as themselves`() {
        assertEquals(RecurrenceWording.EveryNumberOfDays(3), recurrenceWording("FREQ=DAILY;INTERVAL=3"))
        assertEquals(
            RecurrenceWording.OnDaysOfTheWeek(listOf(RecurrenceWeekday.TUESDAY, RecurrenceWeekday.THURSDAY)),
            recurrenceWording("FREQ=WEEKLY;BYDAY=TU,TH"),
        )
        assertEquals(RecurrenceWording.OnDayOfTheMonth(15), recurrenceWording("FREQ=MONTHLY;BYMONTHDAY=15"))
    }

    @Test
    fun `days are read back in week order however they were written`() {
        assertEquals(
            RecurrenceWording.OnDaysOfTheWeek(listOf(RecurrenceWeekday.TUESDAY, RecurrenceWeekday.FRIDAY)),
            recurrenceWording("FREQ=WEEKLY;BYDAY=FR,TU"),
        )
        assertEquals(RecurrenceWording.EveryWeekday, recurrenceWording("FREQ=WEEKLY;BYDAY=FR,MO,TH,TU,WE"))
    }

    @Test
    fun `an interval of one is simply every day`() {
        assertEquals(RecurrenceWording.EveryDay, recurrenceWording("FREQ=DAILY;INTERVAL=1"))
    }

    @Test
    fun `a real rule with no sentence here is shown as it was written`() {
        // Each of these is honoured in full by the calculator. Refusing them
        // would strand a task set up elsewhere.
        assertEquals(RecurrenceWording.AsWritten("FREQ=MONTHLY;BYDAY=3FR"), recurrenceWording("FREQ=MONTHLY;BYDAY=3FR"))
        assertEquals(
            RecurrenceWording.AsWritten("FREQ=MONTHLY;BYMONTHDAY=-1"),
            recurrenceWording("FREQ=MONTHLY;BYMONTHDAY=-1"),
        )
        assertEquals(
            RecurrenceWording.AsWritten("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO"),
            recurrenceWording("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO"),
        )
    }

    @Test
    fun `something that is not a rule is refused`() {
        assertEquals(RecurrenceWording.Unreadable, recurrenceWording("every other tuesday"))
        assertEquals(RecurrenceWording.Unreadable, recurrenceWording("FREQ=FORTNIGHTLY"))
        assertEquals(RecurrenceWording.Unreadable, recurrenceWording("FREQ"))
    }

    @Test
    fun `a rule is recognised as the choice it is whatever order or case it arrives in`() {
        assertEquals(RecurrencePreset.DAILY, RecurrencePreset.of("freq=daily"))
        assertEquals(RecurrencePreset.WEEKDAYS, RecurrencePreset.of("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"))
        assertEquals(RecurrencePreset.YEARLY, RecurrencePreset.of(" FREQ=YEARLY "))
    }

    @Test
    fun `a rule that is not one of the offered choices is none of them`() {
        assertNull(RecurrencePreset.of(null))
        assertNull(RecurrencePreset.of("FREQ=DAILY;INTERVAL=3"))
        assertNull(RecurrencePreset.of("FREQ=WEEKLY;BYDAY=TU,TH"))
    }
}
