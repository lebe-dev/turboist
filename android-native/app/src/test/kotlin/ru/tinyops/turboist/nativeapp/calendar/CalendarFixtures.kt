package ru.tinyops.turboist.nativeapp.calendar

import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.api.CalendarApi
import ru.tinyops.turboist.core.network.dto.CalendarAccountDto
import ru.tinyops.turboist.core.network.dto.CalendarEventDto
import ru.tinyops.turboist.core.network.dto.CalendarEventsDto
import ru.tinyops.turboist.core.network.dto.CalendarSourceDto
import ru.tinyops.turboist.core.network.dto.CalendarStatusDto
import java.io.IOException

/** A timed entry, as the server writes one. */
fun eventDto(
    id: String,
    startsAt: Long,
    endsAt: Long = startsAt + 60 * 60 * 1000,
    title: String = "event $id",
): CalendarEventDto =
    CalendarEventDto(
        id = id,
        sourceName = "Work",
        sourceColor = "#3366cc",
        title = title,
        start = WireTime.format(startsAt),
        end = WireTime.format(endsAt),
    )

/** A whole-day entry, which the server names by day rather than by instant. */
fun allDayDto(
    id: String,
    day: String,
    endDayExclusive: String,
    title: String = "event $id",
): CalendarEventDto =
    CalendarEventDto(
        id = id,
        sourceName = "Personal",
        title = title,
        startDate = day,
        endDate = endDayExclusive,
        allDay = true,
    )

/** An integration that is on, authorised and pointed at one chosen calendar. */
fun connectedStatus(): CalendarStatusDto =
    CalendarStatusDto(
        enabled = true,
        googleConfigured = true,
        accounts = listOf(CalendarAccountDto(id = 1, email = "someone@example.com")),
        sources = listOf(CalendarSourceDto(id = 5, accountId = 1, summary = "Work", selected = true)),
    )

/**
 * A server that answers from a script and counts what it was asked.
 *
 * The counts are the point of most of the checks here: how often the app goes to
 * the network for something it already has is the whole of the caching contract.
 */
class ScriptedCalendarApi(
    var status: CalendarStatusDto = connectedStatus(),
    var events: List<CalendarEventDto> = emptyList(),
) : CalendarApi {
    var statusCalls = 0
    var eventCalls = 0
    var requestedRanges = mutableListOf<Pair<String, String>>()

    /** When set, every call fails with it, the way an unreachable server does. */
    var failWith: Exception? = null

    override suspend fun status(): CalendarStatusDto {
        statusCalls++
        failWith?.let { throw it }
        return status
    }

    override suspend fun events(
        start: String,
        end: String,
    ): CalendarEventsDto {
        eventCalls++
        requestedRanges += start to end
        failWith?.let { throw it }
        return CalendarEventsDto(events)
    }
}

/** The stored copy, held in memory so a check needs no device storage. */
class FakeCalendarCache(
    var entry: CalendarCacheEntry? = null,
) : CalendarCache {
    var writes = 0
    var clears = 0

    override suspend fun read(): CalendarCacheEntry? = entry

    override suspend fun write(entry: CalendarCacheEntry) {
        writes++
        this.entry = entry
    }

    override suspend fun clear() {
        clears++
        entry = null
    }
}

/** The failure an unreachable server produces, as far as this layer can tell. */
fun unreachable(): Exception = IOException("no route to host")
