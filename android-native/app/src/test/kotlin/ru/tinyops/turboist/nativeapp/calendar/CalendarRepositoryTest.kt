package ru.tinyops.turboist.nativeapp.calendar

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.model.view.ViewWindows
import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What the dated screens get from the external calendar, and what it costs.
 *
 * The calendar is the one thing this app reads that is not part of the on-device
 * copy of the user's data, so the checks here are about the three promises that
 * come with that: it is only fetched when the user asked for it, it is fetched
 * rarely, and losing the network changes what is *said* about the entries rather
 * than whether they are shown.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CalendarRepositoryTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")

    // A Thursday, mid-morning, so the day and the week around it are both easy
    // to name in the assertions below.
    private val start: Instant = Instant.parse("2026-03-12T07:00:00Z")

    private var clock: Clock = Clock.fixed(start, zone)
    private val api = ScriptedCalendarApi()
    private val cache = FakeCalendarCache()
    private val preference = MutableStateFlow(CalendarPreference(enabled = true, hidePastEvents = false))

    private fun repository(): CalendarRepository =
        CalendarRepository(
            api = api,
            cache = cache,
            preferences = { preference },
            // Read through a holder so a test can move time on between calls
            // without rebuilding the repository, which is what makes the
            // "asked again later" checks possible.
            clock = MovableClock { clock },
        )

    private fun today(): TimeWindow = ViewWindows.today(clock.instant(), zone)

    private fun advance(by: Duration) {
        clock = Clock.fixed(clock.instant().plus(by), zone)
    }

    /**
     * What a screen showing [window] ends up with.
     *
     * The flow does not wait for the network before its first emission — a dated
     * screen must draw whether or not a calendar server answers — so a check
     * cannot simply take the first value. It collects until everything scheduled
     * has run, which is the settled answer.
     */
    private suspend fun TestScope.shown(
        repository: CalendarRepository,
        window: TimeWindow = today(),
    ): List<CalendarEvent> {
        val seen = mutableListOf<List<CalendarEvent>>()
        val job = launch { repository.observe(window).toList(seen) }
        advanceUntilIdle()
        job.cancel()
        return seen.last()
    }

    @Test
    fun `a calendar the user has not switched on is never asked for`() =
        runTest {
            preference.value = CalendarPreference(enabled = false)
            api.events = listOf(eventDto("a", startsAt = start.toEpochMilli()))

            val shown = shown(repository())

            assertTrue(shown.isEmpty(), "nothing should be shown for a calendar that is off")
            assertEquals(0, api.statusCalls, "a calendar that is off must not be requested")
            assertEquals(0, api.eventCalls)
        }

    @Test
    fun `switching the calendar off takes the stored copy off the device too`() =
        runTest {
            cache.entry =
                CalendarCacheEntry(
                    fetchedAt = clock.millis(),
                    from = today().from,
                    untilExclusive = today().untilExclusive,
                    events = listOf(eventDto("a", startsAt = start.toEpochMilli())),
                )
            preference.value = CalendarPreference(enabled = false)

            shown(repository())

            assertEquals(1, cache.clears, "the kept entries must not outlive the preference that asked for them")
            assertNull(cache.entry)
        }

    @Test
    fun `entries are fetched once and served to every day inside the span`() =
        runTest {
            val morning = start.toEpochMilli()
            val tomorrow = morning + ViewWindows.DAY_MILLIS
            api.events = listOf(eventDto("today", morning), eventDto("tomorrow", tomorrow))
            val repository = repository()

            val onToday = shown(repository)
            val onTomorrow = shown(repository, ViewWindows.tomorrow(clock.instant(), zone))

            assertEquals(listOf("today"), onToday.map { it.id })
            assertEquals(listOf("tomorrow"), onTomorrow.map { it.id })
            assertEquals(1, api.eventCalls, "one span covers every dated screen; a second screen must not refetch")
        }

    @Test
    fun `the fetched span reaches past the week the screens show`() =
        runTest {
            shown(repository())

            val (from, until) = api.requestedRanges.single()
            val week = ViewWindows.week(clock.instant(), zone)
            assertTrue(WireTime.parse(from) <= ViewWindows.dayStart(clock.instant(), zone))
            assertTrue(WireTime.parse(until) > week.untilExclusive, "the span must outlast the week on screen")
        }

    @Test
    fun `a screen opened again soon after is answered without going back to the server`() =
        runTest {
            val repository = repository()
            shown(repository)
            advance(Duration.ofSeconds(30))
            shown(repository)

            assertEquals(1, api.eventCalls)
        }

    @Test
    fun `a screen opened again much later refetches`() =
        runTest {
            val repository = repository()
            shown(repository)
            advance(Duration.ofMinutes(5))
            shown(repository)

            assertEquals(2, api.eventCalls)
        }

    @Test
    fun `an unreachable server leaves the stored entries on screen and says when they were read`() =
        runTest {
            val readAt = clock.millis()
            cache.entry =
                CalendarCacheEntry(
                    fetchedAt = readAt,
                    from = today().from,
                    untilExclusive = today().untilExclusive + 7 * ViewWindows.DAY_MILLIS,
                    events = listOf(eventDto("kept", startsAt = start.toEpochMilli())),
                )
            api.failWith = unreachable()
            val repository = repository()

            val shown = shown(repository)

            assertEquals(listOf("kept"), shown.map { it.id }, "what the device already knows must stay on screen")
            assertEquals(readAt, repository.cachedAsOf.first())
            assertEquals(0, cache.writes, "a failed read must not overwrite the copy it fell back on")
        }

    @Test
    fun `a first launch with no network shows nothing and reports nothing stale`() =
        runTest {
            api.failWith = unreachable()
            val repository = repository()

            assertTrue(shown(repository).isEmpty())
            assertNull(repository.cachedAsOf.first(), "there is no copy to be as of anything")
        }

    @Test
    fun `a server that comes back clears the age note`() =
        runTest {
            cache.entry =
                CalendarCacheEntry(
                    fetchedAt = clock.millis(),
                    from = today().from,
                    untilExclusive = today().untilExclusive,
                    events = listOf(eventDto("kept", startsAt = start.toEpochMilli())),
                )
            api.failWith = unreachable()
            val repository = repository()
            shown(repository)
            assertNotNull(repository.cachedAsOf.first())

            api.failWith = null
            api.events = listOf(eventDto("fresh", startsAt = start.toEpochMilli()))
            advance(Duration.ofMinutes(5))
            val shown = shown(repository)

            assertEquals(listOf("fresh"), shown.map { it.id })
            assertNull(repository.cachedAsOf.first())
        }

    @Test
    fun `only the entries touching the day asked for are returned`() =
        runTest {
            val morning = start.toEpochMilli()
            api.events =
                listOf(
                    eventDto("today", morning),
                    eventDto("in three days", morning + 3 * ViewWindows.DAY_MILLIS),
                )

            val shown = shown(repository())

            assertEquals(listOf("today"), shown.map { it.id })
        }

    @Test
    fun `a whole-day entry lands on the day the server named it for`() =
        runTest {
            api.events = listOf(allDayDto("holiday", day = "2026-03-12", endDayExclusive = "2026-03-13"))

            val shown = shown(repository())

            assertEquals(listOf("holiday"), shown.map { it.id })
            assertTrue(shown.single().allDay)
        }

    @Test
    fun `entries that are over drop off the day when the user asked them to`() =
        runTest {
            preference.value = CalendarPreference(enabled = true, hidePastEvents = true)
            val hourAgo = start.toEpochMilli() - 2 * 60 * 60 * 1000
            api.events =
                listOf(
                    eventDto("finished", startsAt = hourAgo, endsAt = hourAgo + 60 * 60 * 1000),
                    eventDto("later", startsAt = start.toEpochMilli() + 60 * 60 * 1000),
                )

            val shown = shown(repository())

            assertEquals(listOf("later"), shown.map { it.id })
        }

    @Test
    fun `entries that are over stay on the day when the user did not`() =
        runTest {
            val hourAgo = start.toEpochMilli() - 2 * 60 * 60 * 1000
            api.events = listOf(eventDto("finished", startsAt = hourAgo, endsAt = hourAgo + 60 * 60 * 1000))

            val shown = shown(repository())

            assertEquals(listOf("finished"), shown.map { it.id })
        }

    @Test
    fun `nothing is asked of the events endpoint when no calendar is connected`() =
        runTest {
            api.status = connectedStatus().copy(accounts = emptyList(), sources = emptyList())

            val shown = shown(repository())

            assertTrue(shown.isEmpty())
            assertEquals(1, api.statusCalls)
            assertEquals(0, api.eventCalls, "asking for entries of no calendar is a wake-up for an empty answer")
        }
}

/** A clock that reads a holder, so a test can move time on mid-scenario. */
private class MovableClock(
    private val source: () -> Clock,
) : Clock() {
    override fun getZone(): ZoneId = source().zone

    override fun withZone(zone: ZoneId): Clock = source().withZone(zone)

    override fun instant(): Instant = source().instant()
}
