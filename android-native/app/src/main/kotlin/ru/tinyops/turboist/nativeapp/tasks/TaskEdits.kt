package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.sync.write.TaskEdit
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

// Building one field's edit, and reading the dates back out again.
//
// All of it is pure arithmetic over a task and a calendar, kept away from the
// screen for two reasons. It is the part that is easy to get subtly wrong — an
// all-day task and a task due at a time are different statements sharing one
// column — and it is the part that decides how small a queued write is. An edit
// carrying a field the user did not touch is not merely wasteful: two devices
// that changed different fields of the same task would then overwrite each
// other, because the last write to arrive would carry both.

/** Which fields an edit actually touches. Empty means there is nothing to send. */
fun TaskEdit.touchedFields(): List<String> =
    buildList {
        if (title != null) add("title")
        if (description != null) add("description")
        if (priority != null) add("priority")
        if (dueAt != null) add("dueAt")
        if (dueHasTime != null) add("dueHasTime")
        if (deadlineAt != null) add("deadlineAt")
        if (deadlineHasTime != null) add("deadlineHasTime")
        if (dayPart != null) add("dayPart")
        if (planState != null) add("planState")
        if (recurrenceRule != null) add("recurrenceRule")
        if (labels != null) add("labels")
        if (removedAutoLabels.isNotEmpty()) add("removedAutoLabels")
        if (isPrivate != null) add("isPrivate")
        if (isComplex != null) add("isComplex")
    }

/** The day a task is due, as the user's calendar shows it. */
fun Task.dueDate(zone: ZoneId): LocalDate? = dueAt?.let { Instant.ofEpochMilli(it).atZone(zone).toLocalDate() }

/**
 * The time of day a task is due, or `null` when it is due on a day rather than
 * at a moment. The stored instant always has a time in it — midnight, for an
 * all-day task — so the flag is what decides whether there is one to show.
 */
fun Task.dueTime(zone: ZoneId): LocalTime? =
    if (!dueHasTime) null else dueAt?.let { Instant.ofEpochMilli(it).atZone(zone).toLocalTime() }

/** The day a task is due to be finished by. */
fun Task.deadlineDate(zone: ZoneId): LocalDate? =
    deadlineAt?.let { Instant.ofEpochMilli(it).atZone(zone).toLocalDate() }

/** The time of day of a deadline, or `null` when the deadline is a whole day. */
fun Task.deadlineTime(zone: ZoneId): LocalTime? =
    if (!deadlineHasTime) null else deadlineAt?.let { Instant.ofEpochMilli(it).atZone(zone).toLocalTime() }

/**
 * Moves a due date to [date], or empties it.
 *
 * A task already due at a time of day keeps that time when it is moved to
 * another day: the user picked a day, not a new appointment. Emptying the date
 * takes the time with it, because a time of day on no day is not a statement
 * anyone can read.
 */
fun dueDateEdit(
    task: Task,
    date: LocalDate?,
    zone: ZoneId,
): TaskEdit {
    if (date == null) {
        if (task.dueAt == null) return TaskEdit()
        return TaskEdit(dueAt = Clearable.Clear, dueHasTime = if (task.dueHasTime) false else null)
    }
    val at = date.atTime(task.dueTime(zone) ?: LocalTime.MIDNIGHT).atZone(zone).toInstant().toEpochMilli()
    if (at == task.dueAt) return TaskEdit()
    return TaskEdit(dueAt = Clearable.Set(at))
}

/**
 * Gives a due date a time of day, or takes one away.
 *
 * Both directions move the stored instant as well as the flag, so the two never
 * disagree — a task marked as having a time but sitting on midnight would read
 * as due at midnight, which nobody meant. A task with no due date has nothing to
 * put a time on, and the call is a no-op rather than an invented date.
 */
fun dueTimeEdit(
    task: Task,
    time: LocalTime?,
    zone: ZoneId,
): TaskEdit {
    val date = task.dueDate(zone) ?: return TaskEdit()
    val at = date.atTime(time ?: LocalTime.MIDNIGHT).atZone(zone).toInstant().toEpochMilli()
    val hasTime = time != null
    if (at == task.dueAt && hasTime == task.dueHasTime) return TaskEdit()
    return TaskEdit(
        dueAt = if (at == task.dueAt) null else Clearable.Set(at),
        dueHasTime = if (hasTime == task.dueHasTime) null else hasTime,
    )
}

/** Moves a deadline to [date], or empties it. Reads exactly like the due date does. */
fun deadlineDateEdit(
    task: Task,
    date: LocalDate?,
    zone: ZoneId,
): TaskEdit {
    if (date == null) {
        if (task.deadlineAt == null) return TaskEdit()
        return TaskEdit(
            deadlineAt = Clearable.Clear,
            deadlineHasTime = if (task.deadlineHasTime) false else null,
        )
    }
    val at = date.atTime(task.deadlineTime(zone) ?: LocalTime.MIDNIGHT).atZone(zone).toInstant().toEpochMilli()
    if (at == task.deadlineAt) return TaskEdit()
    return TaskEdit(deadlineAt = Clearable.Set(at))
}

/**
 * Sets the rule a task repeats by, or stops it repeating.
 *
 * Stopping is a third state rather than an empty rule, which is why the field is
 * cleared explicitly: a rule left as an empty string would come back from the
 * server as a task that repeats by nothing at all. Surrounding space is dropped
 * before the comparison, so re-saving the same rule with a stray space is not a
 * change and queues nothing.
 */
fun recurrenceEdit(
    task: Task,
    rule: String?,
): TaskEdit {
    val wanted = rule?.trim()?.takeIf { it.isNotEmpty() }
    if (wanted == task.recurrenceRule) return TaskEdit()
    return TaskEdit(recurrenceRule = if (wanted == null) Clearable.Clear else Clearable.Set(wanted))
}

/** Gives a deadline a time of day, or takes one away. */
fun deadlineTimeEdit(
    task: Task,
    time: LocalTime?,
    zone: ZoneId,
): TaskEdit {
    val date = task.deadlineDate(zone) ?: return TaskEdit()
    val at = date.atTime(time ?: LocalTime.MIDNIGHT).atZone(zone).toInstant().toEpochMilli()
    val hasTime = time != null
    if (at == task.deadlineAt && hasTime == task.deadlineHasTime) return TaskEdit()
    return TaskEdit(
        deadlineAt = if (at == task.deadlineAt) null else Clearable.Set(at),
        deadlineHasTime = if (hasTime == task.deadlineHasTime) null else hasTime,
    )
}
