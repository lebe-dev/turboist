package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import java.time.Instant
import java.time.ZoneId

/**
 * When each phase of the day begins and ends, as whole hours of a local day.
 *
 * [endHour] is exclusive, so adjacent phases share a boundary hour without
 * either claiming it twice.
 */
data class DayPartWindow(
    val part: DayPart,
    val startHour: Int,
    val endHour: Int,
)

/**
 * The day's phases, in the order the day reads.
 *
 * The phases are an installation-wide setting on the server rather than a per
 * user one, and they are not part of what a device replicates, so this carries
 * the product's own boundaries. Getting them wrong costs a highlight and the
 * visibility of a phase-scoped announcement, never the membership of a list: a
 * task is filed under a phase by the phase it names, not by the clock.
 *
 * Whatever learns the server's configured boundaries later replaces the value
 * handed to the screens; nothing about the type has to change for that.
 */
data class DayPartSchedule(
    val windows: List<DayPartWindow>,
) {
    /**
     * The phase [now] falls in, or `null` when it falls in none — late at night,
     * or before the day's first phase has started. Null is a real answer: it is
     * what makes a phase-scoped announcement stay hidden outside its phase.
     */
    fun activeAt(
        now: Instant,
        zone: ZoneId,
    ): DayPart? {
        val hour = now.atZone(zone).hour
        return windows.firstOrNull { hour >= it.startHour && hour < it.endHour }?.part
    }

    companion object {
        /** The product's own boundaries, and what a device uses until it is told otherwise. */
        val Default: DayPartSchedule =
            DayPartSchedule(
                listOf(
                    DayPartWindow(DayPart.MORNING, 9, 13),
                    DayPartWindow(DayPart.AFTERNOON, 13, 17),
                    DayPartWindow(DayPart.EVENING, 17, 22),
                ),
            )
    }
}
