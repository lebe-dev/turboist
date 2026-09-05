package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import java.time.Clock
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The clock the dated screens read.
 *
 * They read it through a ticking flow rather than once, because a day view left
 * open has to notice two moments on its own: midnight, when the list it shows
 * stops being today's, and the boundary between two phases of the day, when the
 * highlight moves and a phase-scoped announcement appears or goes. Neither
 * arrives as a change to the replica, so nothing else would tell the screen.
 *
 * The tick is a minute, which is the coarsest interval that still lands both
 * boundaries as they happen.
 */
@Singleton
class DayClock
    @Inject
    constructor(
        private val clock: Clock,
    ) {
        /** The zone the user's days are measured in. */
        val zone: ZoneId get() = clock.zone

        /** The current instant now, and again every minute for as long as it is collected. */
        fun ticks(): Flow<Instant> =
            flow {
                while (true) {
                    emit(clock.instant())
                    delay(TICK_MILLIS)
                }
            }

        /** This moment, for the one-off answers a ticking flow cannot give. */
        fun now(): Instant = clock.instant()

        /** The calendar day it is at this moment, where the user is. */
        fun today(): LocalDate = clock.instant().atZone(clock.zone).toLocalDate()

        /**
         * The current calendar day, and again each time it turns over.
         *
         * A dated list names one of its blocks "Today", so a screen left open
         * over midnight has to be told that the word belongs to a different day
         * now. Only the turnover is emitted, not every tick.
         */
        fun days(): Flow<LocalDate> = ticks().map { it.atZone(clock.zone).toLocalDate() }.distinctUntilChanged()

        private companion object {
            const val TICK_MILLIS = 60_000L
        }
    }
