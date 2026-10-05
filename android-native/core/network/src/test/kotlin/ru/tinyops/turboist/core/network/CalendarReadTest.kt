package ru.tinyops.turboist.core.network

import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.mapping.toCalendarConnection
import ru.tinyops.turboist.core.network.mapping.toCalendarEvent
import java.time.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The two calendar reads, over the real interceptor chain.
 *
 * Only reads exist to check: connecting an account and choosing calendars happen
 * where an OAuth redirect can land, which is not here. What matters at this layer
 * is that the range goes out in the form the server parses, and that an answer
 * comes back as values the screens can lay out — in particular that a whole-day
 * entry keeps the *days* the provider named rather than being reduced to the
 * instant beside them.
 */
class CalendarReadTest {
    @Test
    fun `the status is read from the calendars collection`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(
                    jsonResponse(
                        200,
                        """
                        {"enabled":true,"googleConfigured":true,
                         "accounts":[{"id":1,"provider":"google","email":"someone@example.com"}],
                         "sources":[{"id":5,"accountId":1,"summary":"Work","selected":true},
                                    {"id":6,"accountId":1,"summary":"Holidays","selected":false}]}
                        """.trimIndent(),
                    ),
                )

                val status = fixture.network.calendars.status()

                assertEquals("/api/v1/calendars/", fixture.server.takeRequest().url.encodedPath)
                val connection = status.toCalendarConnection()
                assertEquals(1, connection.connectedAccounts)
                assertEquals(1, connection.selectedSources, "only the calendars the user chose count")
                assertTrue(connection.showsEvents)
            }
        }

    @Test
    fun `a connected account with nothing chosen has nothing to show`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(
                    jsonResponse(
                        200,
                        """{"enabled":true,"accounts":[{"id":1}],"sources":[{"id":5,"selected":false}]}""",
                    ),
                )

                assertFalse(fixture.network.calendars.status().toCalendarConnection().showsEvents)
            }
        }

    @Test
    fun `the range goes out as two instants on the query string`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(jsonResponse(200, """{"items":[]}"""))

                fixture.network.calendars.events("2026-03-12T00:00:00.000Z", "2026-03-19T00:00:00.000Z")

                val request = fixture.server.takeRequest()
                assertEquals("/api/v1/calendars/events", request.url.encodedPath)
                assertEquals("2026-03-12T00:00:00.000Z", request.url.queryParameter("start"))
                assertEquals("2026-03-19T00:00:00.000Z", request.url.queryParameter("end"))
            }
        }

    @Test
    fun `a timed entry keeps its moments and its calendar's colour`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(
                    jsonResponse(
                        200,
                        """
                        {"items":[{"id":"e1","sourceName":"Work","sourceColor":"#3366cc","title":"Standup",
                                   "start":"2026-03-12T09:00:00.000Z","end":"2026-03-12T09:15:00.000Z",
                                   "allDay":false,"htmlLink":"https://calendar.example/e1"}]}
                        """.trimIndent(),
                    ),
                )

                val event = fixture.network.calendars.events("a", "b").items.single().toCalendarEvent()

                assertEquals("Standup", event.title)
                assertEquals("#3366cc", event.sourceColour)
                assertEquals("https://calendar.example/e1", event.link)
                assertFalse(event.allDay)
                assertNull(event.startDay, "a timed entry has no day of its own to be filed under")
                assertEquals(WireTime.parse("2026-03-12T09:00:00.000Z"), event.startsAt)
                assertEquals(WireTime.parse("2026-03-12T09:15:00.000Z"), event.endsAt)
            }
        }

    @Test
    fun `a whole-day entry keeps the days the provider named, end exclusive`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(
                    jsonResponse(
                        200,
                        """
                        {"items":[{"id":"e2","title":"Away","allDay":true,
                                   "start":"2026-03-11T21:00:00.000Z","end":"2026-03-13T21:00:00.000Z",
                                   "startDate":"2026-03-12","endDate":"2026-03-14"}]}
                        """.trimIndent(),
                    ),
                )

                val event = fixture.network.calendars.events("a", "b").items.single().toCalendarEvent()

                assertTrue(event.allDay)
                assertEquals(LocalDate.of(2026, 3, 12), event.startDay)
                assertEquals(LocalDate.of(2026, 3, 14), event.endDay)
            }
        }

    @Test
    fun `a whole-day entry with an unreadable day falls back on its instant`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(
                    jsonResponse(
                        200,
                        """
                        {"items":[{"id":"e3","title":"Away","allDay":true,
                                   "start":"2026-03-11T21:00:00.000Z","startDate":"not a date"}]}
                        """.trimIndent(),
                    ),
                )

                val event = fixture.network.calendars.events("a", "b").items.single().toCalendarEvent()

                assertNull(event.startDay)
                assertEquals(WireTime.parse("2026-03-11T21:00:00.000Z"), event.startsAt)
            }
        }

    @Test
    fun `a grant the provider withdrew comes back as a refusal the caller can recognise`() =
        runTest {
            NetworkFixture().use { fixture ->
                fixture.server.enqueue(errorResponse(409, ApiErrorCodes.CALENDAR_REAUTH_REQUIRED))

                val failure = assertFailsWith<ApiException.Business> { fixture.network.calendars.events("a", "b") }

                assertEquals(ApiErrorCodes.CALENDAR_REAUTH_REQUIRED, failure.code)
            }
        }
}
