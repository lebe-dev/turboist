package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Update
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/** How loudly a row reports that its task keeps being put off. */
enum class PostponeTone {
    /** Put off twice: worth a glance. */
    WARNING,

    /** Put off three times or more: a habit, not a bad day. */
    ALARM,

    /** The task is finished; the count is history and is drawn as such. */
    MUTED,
}

/**
 * The tone for a task put off [count] times, or null when it is not worth showing.
 *
 * Mirrors the web client's badge: a single postponement is a normal day, the
 * second turns amber, and from the third on it turns red.
 */
fun postponeTone(
    count: Int,
    completed: Boolean,
): PostponeTone? =
    when {
        count < POSTPONE_WORTH_SHOWING -> null
        completed -> PostponeTone.MUTED
        count == POSTPONE_WORTH_SHOWING -> PostponeTone.WARNING
        else -> PostponeTone.ALARM
    }

/** A clock-with-arrow and the count, coloured by [tone] — the web's postpone badge. */
@Composable
internal fun PostponeBadge(
    count: Int,
    tone: PostponeTone,
) {
    val color =
        when (tone) {
            PostponeTone.WARNING -> TurboistTheme.accents.postponedTwice
            PostponeTone.ALARM -> TurboistTheme.accents.postponedOften
            PostponeTone.MUTED -> MaterialTheme.colorScheme.onSurfaceVariant
        }
    val description = stringResource(R.string.task_postponedTimes, count)
    Row(
        modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = description },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Update,
            contentDescription = null,
            tint = color,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = count.toString(),
            style = MaterialTheme.typography.labelMedium,
            fontWeight = FontWeight.Medium,
            color = color,
        )
    }
}

/**
 * How many postponements are worth showing. One is a normal day; a habit of
 * putting the same task off is the thing worth noticing.
 */
private const val POSTPONE_WORTH_SHOWING = 2
