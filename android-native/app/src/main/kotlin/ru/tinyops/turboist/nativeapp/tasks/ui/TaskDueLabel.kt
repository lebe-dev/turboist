package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.core.model.view.dayOf
import ru.tinyops.turboist.core.model.view.relativeDay
import ru.tinyops.turboist.nativeapp.R
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * How a due date reads on a row: a named day where the day has a name, a short
 * date otherwise, followed by the time when the task was given one.
 *
 * A task due "on the 9th" and a task due "at 12:34 on the 9th" are different
 * statements, and only the second one has a time to show. Printing midnight for
 * the first would invent a deadline the user never set.
 */
@Composable
fun dueLabel(
    task: Task,
    zone: ZoneId,
    today: LocalDate,
): String? {
    val dueAt = task.dueAt ?: return null
    val day = dayOf(dueAt, zone)
    val named =
        when (relativeDay(day, today)) {
            RelativeDay.TODAY -> stringResource(R.string.common_today)
            RelativeDay.TOMORROW -> stringResource(R.string.common_tomorrow)
            RelativeDay.YESTERDAY -> stringResource(R.string.common_yesterday)
            RelativeDay.OTHER -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
        }
    if (!task.dueHasTime) return named
    val time =
        Instant.ofEpochMilli(dueAt)
            .atZone(zone)
            .toLocalTime()
            .format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))
    return "$named · $time"
}

/** The heading of a day block: a named day where there is one, a full date otherwise. */
@Composable
fun dayHeadingLabel(
    day: LocalDate?,
    relative: RelativeDay?,
): String =
    when {
        day == null -> stringResource(R.string.common_noDate)
        relative == RelativeDay.TODAY -> stringResource(R.string.common_today)
        relative == RelativeDay.TOMORROW -> stringResource(R.string.common_tomorrow)
        relative == RelativeDay.YESTERDAY -> stringResource(R.string.common_yesterday)
        else -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL))
    }
