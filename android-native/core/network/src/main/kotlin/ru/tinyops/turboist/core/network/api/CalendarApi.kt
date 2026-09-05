package ru.tinyops.turboist.core.network.api

import retrofit2.http.GET
import retrofit2.http.Query
import ru.tinyops.turboist.core.network.dto.CalendarEventsDto
import ru.tinyops.turboist.core.network.dto.CalendarStatusDto

/**
 * The external calendar, read-only.
 *
 * Two calls and no writes, on purpose. Connecting an account, choosing which
 * calendars are shown and turning the integration off are done where the OAuth
 * redirect can complete, which is the browser; this client only looks at the
 * result. Nothing here takes an idempotency key either, because nothing here
 * changes anything.
 *
 * Entries are never stored on the server, so neither call is part of the change
 * history a device catches up on: both are ordinary online reads whose answers
 * this app keeps only so the days on screen still read correctly with no
 * network.
 */
interface CalendarApi {
    /** Whether the integration is on, which accounts are authorised, and which of their calendars are shown. */
    @GET("api/v1/calendars/")
    suspend fun status(): CalendarStatusDto

    /**
     * The entries between two instants, both in the API's UTC millisecond form
     * and both required. The server refuses a range wider than three months and
     * answers with an empty list whenever the integration is off or unconnected,
     * so a caller never has to ask whether it is worth asking.
     */
    @GET("api/v1/calendars/events")
    suspend fun events(
        @Query("start") start: String,
        @Query("end") end: String,
    ): CalendarEventsDto
}
