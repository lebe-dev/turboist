package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.Task
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

/**
 * Whether the title ends in a counter such as `Read a chapter (3)` — the mark of
 * a routine whose next copy is made for the following day.
 */
fun Task.hasTitleCounter(): Boolean = TITLE_COUNTER.matches(title)

private val TITLE_COUNTER = Regex("^.+ \\(\\d+\\)$")

/**
 * The due date a copy of this task gets when it is placed on [day]: the same
 * time of day, or the start of the day for a task due on a day alone.
 */
fun Task.copyDueAt(
    day: LocalDate,
    zone: ZoneId,
): Long = day.atTime(dueTime(zone) ?: LocalTime.MIDNIGHT).atZone(zone).toInstant().toEpochMilli()
