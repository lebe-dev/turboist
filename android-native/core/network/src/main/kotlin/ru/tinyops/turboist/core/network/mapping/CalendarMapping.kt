package ru.tinyops.turboist.core.network.mapping

import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.model.calendar.CalendarConnection
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.core.network.dto.CalendarEventDto
import ru.tinyops.turboist.core.network.dto.CalendarStatusDto
import java.time.LocalDate
import java.time.format.DateTimeParseException

/**
 * Turns the calendar payloads into the values the screens read.
 *
 * The one conversion worth naming is the all-day day pair. The provider reports
 * a whole day as a bare `yyyy-MM-dd` with an **exclusive** end, alongside an
 * instant that stands for the day in whatever zone the provider chose. The bare
 * days are the truthful part, so they are parsed and kept; a day that cannot be
 * read becomes absent rather than a guess, and the entry then falls back on its
 * instant like a timed one.
 */
fun CalendarEventDto.toCalendarEvent(): CalendarEvent =
    CalendarEvent(
        id = id,
        title = title,
        sourceName = sourceName,
        sourceColour = sourceColor,
        startsAt = WireTime.parseOrNull(start) ?: 0L,
        endsAt = WireTime.parseOrNull(end) ?: 0L,
        startDay = if (allDay) localDate(startDate) else null,
        endDay = if (allDay) localDate(endDate) else null,
        allDay = allDay,
        link = htmlLink,
    )

/** The integration's state, reduced to the two counts the app actually branches on. */
fun CalendarStatusDto.toCalendarConnection(): CalendarConnection =
    CalendarConnection(
        enabled = enabled,
        connectedAccounts = accounts.size,
        selectedSources = sources.count { it.selected },
    )

private fun localDate(raw: String?): LocalDate? {
    if (raw.isNullOrBlank()) return null
    return try {
        LocalDate.parse(raw.trim())
    } catch (_: DateTimeParseException) {
        null
    }
}
