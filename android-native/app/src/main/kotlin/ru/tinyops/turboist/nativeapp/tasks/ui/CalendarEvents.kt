package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.nativeapp.R
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * The appointments on a day, drawn under the heading of the block they fall in.
 *
 * They are deliberately not rows. A row is something the user acts on — ticks,
 * swipes, opens, picks in a selection — and none of that is true of an entry
 * that lives in someone else's calendar and is read-only here. So the entries
 * are one quiet band: no checkbox, no gesture, nothing to tap. Making them look
 * like tasks would promise actions the app cannot carry out.
 */
@Composable
fun CalendarEventBand(
    events: List<CalendarEvent>,
    zone: ZoneId,
    modifier: Modifier = Modifier,
) {
    if (events.isEmpty()) return
    Column(
        modifier =
            modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 4.dp)
                .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(8.dp))
                .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        for (event in events) CalendarEventLine(event, zone)
    }
}

/**
 * One appointment: the calendar's own colour, when it is, and what it is.
 *
 * The line is announced as a single phrase rather than as two fragments, because
 * that is what it is — a time and what it is for are not two things a reader
 * wants separately. The colour stripe says nothing at all: it repeats which
 * calendar the entry came from, which the entry's own words already carry.
 */
@Composable
private fun CalendarEventLine(
    event: CalendarEvent,
    zone: ZoneId,
) {
    val allDay = stringResource(R.string.calendar_allDay)
    val time = eventTimeLabel(event, zone, allDay)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) {},
    ) {
        Box(
            modifier =
                Modifier
                    .width(3.dp)
                    .height(14.dp)
                    .background(sourceColour(event), CircleShape),
        )
        Text(
            text = time,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = event.title,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/**
 * The line a screen shows when its appointments could not be refreshed.
 *
 * It says when they were read and nothing else. There is no retry and no
 * warning: the tasks on the screen are current whatever the calendar did, and
 * turning a calendar the app could not reach into an error the user has to
 * dismiss would be noise about someone else's system.
 */
@Composable
fun CalendarAsOfNote(
    asOfEpochMillis: Long,
    zone: ZoneId,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.CloudOff,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = stringResource(R.string.native_calendar_as_of, readAtLabel(asOfEpochMillis, zone)),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * When an appointment is, in words.
 *
 * A whole-day entry has no times to show, and an entry whose end the provider
 * left out or put before its start is shown by its start alone rather than as a
 * span that runs backwards.
 */
internal fun eventTimeLabel(
    event: CalendarEvent,
    zone: ZoneId,
    allDayLabel: String,
): String {
    if (event.allDay) return allDayLabel
    val start = timeOfDay(event.startsAt, zone)
    if (event.endsAt <= event.startsAt) return start
    return start + "–" + timeOfDay(event.endsAt, zone)
}

private fun timeOfDay(
    epochMillis: Long,
    zone: ZoneId,
): String = TIME_FORMAT.format(Instant.ofEpochMilli(epochMillis).atZone(zone))

private fun readAtLabel(
    epochMillis: Long,
    zone: ZoneId,
): String = READ_AT_FORMAT.format(Instant.ofEpochMilli(epochMillis).atZone(zone))

/**
 * The calendar's own colour, or the theme's outline when the provider named none
 * or named something unreadable. A stripe that cannot be drawn must not take the
 * line with it.
 */
@Composable
private fun sourceColour(event: CalendarEvent): Color =
    parseHexColour(event.sourceColour) ?: MaterialTheme.colorScheme.outline

internal fun parseHexColour(raw: String): Color? {
    val text = raw.trim().removePrefix("#")
    val argb =
        when (text.length) {
            // Six digits are the form every provider uses; the alpha is ours to
            // supply, and it is always opaque.
            6 -> text.toLongOrNull(radix = 16)?.or(0xFF000000L)
            8 -> text.toLongOrNull(radix = 16)
            else -> null
        } ?: return null
    return Color(argb.toInt())
}

private val TIME_FORMAT: DateTimeFormatter = DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT)

private val READ_AT_FORMAT: DateTimeFormatter =
    DateTimeFormatter.ofLocalizedDateTime(FormatStyle.SHORT, FormatStyle.SHORT)
