package ru.tinyops.turboist.nativeapp.calendar

import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.core.model.calendar.sortCalendarEvents
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.network.api.CalendarApi
import ru.tinyops.turboist.core.network.mapping.toCalendarConnection
import ru.tinyops.turboist.core.network.mapping.toCalendarEvent
import java.time.Clock
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The external calendar, as the dated screens see it.
 *
 * Three rules shape everything here, and they are the reason the calendar has a
 * repository of its own rather than a place in the replica:
 *
 * 1. **It is read, never written.** No screen can change an entry, so there is no
 *    optimistic path, no queued write and no conflict to resolve. What the server
 *    said is all there is.
 * 2. **It is fetched online and kept only as a fallback.** One span covering
 *    every dated screen is requested at most once every few minutes; the answer
 *    is stored so the same days still read correctly with no network, and the
 *    screens are told when what they are showing is that stored copy.
 * 3. **A failure is silent.** A calendar the app could not reach is not an error
 *    the user has to dismiss — their tasks are unaffected and the days still
 *    render. The only thing said about it is *when* the entries on screen were
 *    read, and that only once they are stale.
 *
 * The entries are filtered as each emission is built, so a timed entry that ends
 * while a screen sits open stays on it until the next read. That is deliberate:
 * re-evaluating it continuously would mean waking every open screen once a
 * minute to remove one line.
 */
@Singleton
class CalendarRepository
    @Inject
    constructor(
        private val api: CalendarApi,
        private val cache: CalendarCache,
        private val preferences: CalendarPreferences,
        private val clock: Clock,
    ) {
        private val stored = MutableStateFlow<CalendarCacheEntry?>(null)
        private val staleSince = MutableStateFlow<Long?>(null)
        private val lock = Mutex()
        private var loadedFromDisk = false
        private var lastAttemptAt = 0L

        /**
         * When the entries on screen were read, and `null` while they are current.
         *
         * Non-null means exactly one thing: a refresh was tried and did not
         * reach the server, so what is on screen is the copy from this moment.
         * It is the whole of what the app says about a calendar it cannot reach.
         */
        val cachedAsOf: Flow<Long?> = staleSince.asStateFlow()

        /**
         * The entries inside [window], and again whenever they change.
         *
         * Collecting asks for a refresh, which is what makes opening a dated
         * screen the trigger; the ask is ignored when the copy in hand is recent
         * enough and already covers the days in question.
         *
         * The refresh runs *beside* the emissions and never in front of them.
         * A dated screen is a query over the device's own data, and it must draw
         * whether or not somebody else's calendar server can be reached — waiting
         * on that request before the first emission would let a slow calendar hold
         * the user's tasks off the screen.
         */
        @OptIn(ExperimentalCoroutinesApi::class)
        fun observe(window: TimeWindow): Flow<List<CalendarEvent>> =
            preferences.observe().distinctUntilChanged().flatMapLatest { preference ->
                if (!preference.enabled) {
                    forget()
                    return@flatMapLatest flowOf(emptyList<CalendarEvent>())
                }
                channelFlow {
                    loadStored()
                    launch { refresh() }
                    stored.collect { entry -> send(visible(entry, window, preference)) }
                }
            }

        /**
         * Fetches the span covering every dated screen, unless the copy in hand
         * already does.
         *
         * A failed attempt counts against the interval exactly like a successful
         * one. Otherwise a device with no network would try again on every screen
         * that is opened, which is the situation where retrying costs the most
         * and gains the least.
         */
        private suspend fun refresh() {
            lock.withLock {
                readStoredOnce()
                val now = clock.millis()
                val coverage = CalendarWindows.coverage(clock.instant(), clock.zone)
                val copy = stored.value
                val recent = now - lastAttemptAt < REFRESH_INTERVAL_MILLIS
                if (copy != null && copy.covers(coverage) && recent) return
                lastAttemptAt = now
                try {
                    val status = api.status()
                    val connection = status.toCalendarConnection()
                    // Nothing to ask for when no calendar is connected or none is
                    // chosen: the server would answer with an empty list anyway,
                    // and this saves the phone a second round trip to hear it.
                    val events =
                        if (connection.showsEvents) {
                            api.events(WireTime.format(coverage.from), WireTime.format(coverage.untilExclusive)).items
                        } else {
                            emptyList()
                        }
                    val entry =
                        CalendarCacheEntry(
                            fetchedAt = now,
                            from = coverage.from,
                            untilExclusive = coverage.untilExclusive,
                            events = events,
                            status = status,
                        )
                    cache.write(entry)
                    stored.value = entry
                    staleSince.value = null
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    // Deliberately not surfaced as a failure, whatever went wrong.
                    // The days on screen are still correct; all that changes is
                    // that their calendar entries are as of when they were last
                    // read. An unreachable server and a server that answered with
                    // something unreadable call for the same thing: show what is
                    // already here, say when it was read, try again later.
                    Log.i(TAG, "Calendar entries could not be refreshed; showing the copy already on the device", e)
                    staleSince.value = stored.value?.fetchedAt
                }
            }
        }

        /** Reads the stored copy the first time anything asks for entries. */
        private suspend fun loadStored() {
            lock.withLock { readStoredOnce() }
        }

        /** The read itself. The caller holds the lock, so the copy is read once and only once. */
        private suspend fun readStoredOnce() {
            if (loadedFromDisk) return
            loadedFromDisk = true
            stored.value = cache.read()
        }

        /**
         * Drops everything the app holds about the calendar.
         *
         * Called when the user turns the calendar off, which has to take their
         * entries off the device rather than merely off the screen.
         */
        private suspend fun forget() {
            lock.withLock {
                loadedFromDisk = true
                lastAttemptAt = 0L
                stored.value = null
                staleSince.value = null
                cache.clear()
            }
        }

        /** The entries of [entry] that belong on a screen showing [window]. */
        private fun visible(
            entry: CalendarCacheEntry?,
            window: TimeWindow,
            preference: CalendarPreference,
        ): List<CalendarEvent> {
            if (entry == null) return emptyList()
            val now = clock.instant()
            val zone = clock.zone
            val events =
                entry.events
                    .map { it.toCalendarEvent() }
                    .filter { it.overlaps(window, zone) }
                    .filterNot { preference.hidePastEvents && it.hasEnded(now, zone) }
            return sortCalendarEvents(events)
        }

        private companion object {
            const val TAG = "Calendar"

            /**
             * How long a fetched span is treated as current.
             *
             * Matches what the web client does, and for the same reason: the
             * server caches the provider's answer for minutes at a time, so
             * asking more often mostly re-reads the server's own copy at the cost
             * of a radio wake-up.
             */
            const val REFRESH_INTERVAL_MILLIS = 2L * 60L * 1000L
        }
    }
