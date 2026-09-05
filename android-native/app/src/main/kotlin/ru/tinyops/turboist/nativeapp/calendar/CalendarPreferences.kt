package ru.tinyops.turboist.nativeapp.calendar

import kotlinx.coroutines.flow.Flow

/**
 * What the user asked for, as far as the calendar is concerned.
 *
 * @property enabled whether calendar entries are shown beside tasks at all. When
 *   this is off nothing is requested, nothing is kept, and anything already kept
 *   is thrown away — a switch the user turned off must not leave their calendar
 *   sitting on the device.
 * @property hidePastEvents whether a timed entry that is already over drops out
 *   of the day it was on.
 */
data class CalendarPreference(
    val enabled: Boolean = false,
    val hidePastEvents: Boolean = true,
)

/**
 * Where those preferences are read from.
 *
 * A seam rather than a direct read, so the calendar layer never reaches into the
 * replica: the preferences arrive as plain values from whoever already knows
 * them, and the calendar keeps its promise of touching no replicated table.
 */
fun interface CalendarPreferences {
    /** The current preference, and again whenever the user changes it. */
    fun observe(): Flow<CalendarPreference>
}
