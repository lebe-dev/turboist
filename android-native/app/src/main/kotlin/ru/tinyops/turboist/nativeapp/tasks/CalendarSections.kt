package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.core.model.view.relativeDay
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

// Where the user's external calendar lands on a dated list.
//
// A screen builds its blocks from the replica first and the calendar is folded
// in afterwards, never the other way round. That order is the guarantee: the
// blocks a list has are decided entirely by the tasks in it, and entries can
// only ever attach to a block or add one — they can never remove a block, change
// its rows, or alter the order the queries decided on.
//
// An entry can add a block, and must be able to: a morning with two meetings and
// no tasks is still a morning that has something in it, and a screen that dropped
// it would be telling the user their morning is free.

// The order the day reads in, which is the order the task grouping uses too.
// `NONE` is last: it is where anything with no phase of its own goes, not a
// fourth phase after the evening.
private val PHASE_ORDER = listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE)

/**
 * Folds calendar entries into a day's phase blocks.
 *
 * An entry is filed under the phase its start falls in, by the same boundaries
 * the day's highlight uses. A whole-day entry has no time to place it by, so it
 * goes to "any time" — which is exactly what it is.
 *
 * @param activePart the phase the clock is in, so a block that exists only
 *   because of its entries is still highlighted when it is the phase happening
 *   now. When the merged list ends up with a single block that block is marked
 *   active whatever the clock says, matching what a phase-cut list does with its
 *   tasks: there is nothing else on screen for the highlight to tell it apart
 *   from, and greying out the only block would say the day was over.
 */
fun withPhaseEvents(
    sections: List<TaskListSection>,
    events: List<CalendarEvent>,
    zone: ZoneId,
    schedule: DayPartSchedule,
    activePart: DayPart?,
): List<TaskListSection> {
    if (events.isEmpty()) return sections
    val byPhase = events.groupBy { phaseOf(it, zone, schedule) }
    val existing =
        sections.mapNotNull { block -> (block.heading as? SectionHeading.Phase)?.let { it.part to block } }.toMap()
    val untouched = sections.filter { it.heading !is SectionHeading.Phase }
    val phases =
        PHASE_ORDER.mapNotNull { part ->
            val block = existing[part]
            val entries = byPhase[part].orEmpty()
            when {
                block != null -> block.copy(events = entries)
                entries.isEmpty() -> null
                else ->
                    TaskListSection(
                        key = "phase-" + part.name,
                        heading = SectionHeading.Phase(part, part == activePart),
                        rows = emptyList(),
                        events = entries,
                    )
            }
        }
    val single = phases.singleOrNull()
    val onlyPhase = single?.heading as? SectionHeading.Phase
    if (single != null && onlyPhase != null) return untouched + single.copy(heading = onlyPhase.copy(active = true))
    return untouched + phases
}

/**
 * Folds calendar entries into a multi-day list's day blocks.
 *
 * A day that holds only entries gets a block of its own, in date order among the
 * days that hold tasks. The undated block, if the list has one, stays last: it
 * is where work with no date goes, and no calendar entry can land there — an
 * entry always names a day.
 */
fun withDayEvents(
    sections: List<TaskListSection>,
    events: List<CalendarEvent>,
    zone: ZoneId,
    today: LocalDate,
): List<TaskListSection> {
    if (events.isEmpty()) return sections
    val byDay = events.groupBy { it.dayIn(zone) }
    val untouched = sections.filter { it.heading !is SectionHeading.Day }
    val dayBlocks = sections.filter { it.heading is SectionHeading.Day }
    val dated = dayBlocks.mapNotNull { block -> (block.heading as SectionHeading.Day).day?.let { it to block } }.toMap()
    val undated = dayBlocks.filter { (it.heading as SectionHeading.Day).day == null }
    val days =
        (dated.keys + byDay.keys).sorted().map { day ->
            val entries = byDay[day].orEmpty()
            dated[day]?.copy(events = entries)
                ?: TaskListSection(
                    key = "day-$day",
                    heading = SectionHeading.Day(day, relativeDay(day, today)),
                    rows = emptyList(),
                    events = entries,
                )
        }
    return untouched + days + undated
}

/**
 * The phase an entry belongs to.
 *
 * A timed entry outside every phase — one starting before the day's first phase
 * or after its last — falls to "any time" rather than being dropped. It is on
 * that day and has to be visible on it.
 */
private fun phaseOf(
    event: CalendarEvent,
    zone: ZoneId,
    schedule: DayPartSchedule,
): DayPart {
    if (event.allDay) return DayPart.NONE
    return schedule.activeAt(Instant.ofEpochMilli(event.startsAt), zone) ?: DayPart.NONE
}
