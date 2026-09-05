package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ProjectFilter
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage

/**
 * The named colours a project or a context can be given.
 *
 * The set is the server's: it accepts one of these ten names or a `#rrggbb`
 * value, and nothing else. They are spelled out here rather than derived from
 * the theme because they are the user's choice about one project, not a role in
 * the design — two projects painted "blue" must look alike whatever the theme
 * around them is doing.
 */
private val NAMED_COLORS: Map<String, Color> =
    mapOf(
        "red" to Color(0xFFE5484D),
        "orange" to Color(0xFFF76B15),
        "yellow" to Color(0xFFFFB224),
        "green" to Color(0xFF30A46C),
        "teal" to Color(0xFF12A594),
        "blue" to Color(0xFF0091FF),
        "purple" to Color(0xFF8E4EC6),
        "pink" to Color(0xFFD6409F),
        "grey" to Color(0xFF8B8D98),
        "brown" to Color(0xFFAD7F58),
    )

/**
 * The palette a coloured record is picked from, in the order it is offered.
 *
 * The names are the server's, and every record it colours — a project, a
 * context, a label — is coloured from this one set. A second list somewhere else
 * would drift the first time the server accepted one more.
 */
val NAMED_COLOR_CHOICES: List<String> = NAMED_COLORS.keys.toList()

/**
 * The colour a project is drawn in, or `null` when it has none the app can read.
 *
 * A value this build cannot make sense of is treated as no colour at all rather
 * than as black: a newer server may accept a spelling this one does not know,
 * and painting the row a colour the user never chose is worse than leaving it in
 * the theme's own.
 */
fun projectTint(color: String): Color? {
    val value = color.trim()
    if (value.isEmpty()) return null
    NAMED_COLORS[value.lowercase()]?.let { return it }
    if (!value.startsWith("#") || value.length != HEX_COLOR_LENGTH) return null
    val rgb = value.substring(1).toLongOrNull(radix = 16) ?: return null
    return Color(rgb or OPAQUE)
}

private const val HEX_COLOR_LENGTH = 7
private const val OPAQUE = 0xFF000000L

/** The wording of a narrowing chip on the projects screen. */
@StringRes
fun filterLabel(filter: ProjectFilter): Int =
    when (filter) {
        ProjectFilter.ALL -> R.string.page_projects_filterAll
        ProjectFilter.GENERIC -> R.string.page_projects_filterGeneric
        ProjectFilter.SOFTWARE -> R.string.page_projects_filterSoftware
        ProjectFilter.ARCHIVED -> R.string.page_projects_filterArchived
        ProjectFilter.CANCELLED -> R.string.page_projects_filterCancelled
        ProjectFilter.COMPLETED -> R.string.page_projects_filterCompleted
    }

/**
 * The badge a project carries when it is no longer open, or `null` while it is.
 * An open project needs no badge — that is what every project on the screen is
 * unless it says otherwise.
 */
@StringRes
fun statusLabel(status: ProjectStatus): Int? =
    when (status) {
        ProjectStatus.OPEN -> null
        ProjectStatus.COMPLETED -> R.string.project_statusCompleted
        ProjectStatus.ARCHIVED -> R.string.project_statusArchived
        ProjectStatus.CANCELLED -> R.string.project_statusCancelled
        // A state this build does not recognise still has to read as "not open".
        // Saying so plainly is better than drawing it as ordinary work, which
        // would invite the user to act on it as if nothing had happened to it.
        ProjectStatus.UNKNOWN -> R.string.native_project_statusUnknown
    }

/**
 * The name of one slot of the daily plan, or `null` for a slot this build does
 * not know. Such a project is still shown as planned — the mark is on it — but
 * it is not given the name of a slot it may not be in.
 */
@StringRes
fun troikiLabel(category: TroikiCategory): Int? =
    when (category) {
        TroikiCategory.IMPORTANT -> R.string.troiki_section_important
        TroikiCategory.MEDIUM -> R.string.troiki_section_medium
        TroikiCategory.REST -> R.string.troiki_section_rest
        TroikiCategory.UNKNOWN -> null
    }

/** What a project screen says back, as a sentence. */
@Composable
fun projectMessageText(message: ProjectMessage): String =
    when (message) {
        is ProjectMessage.Blocked -> stringResource(R.string.task_toast_blockedCannotComplete)
        is ProjectMessage.PinLimitReached -> stringResource(R.string.native_project_pinLimit, message.limit)
        is ProjectMessage.TroikiSlotFull -> stringResource(R.string.native_project_troikiFull, message.capacity)
        is ProjectMessage.Failed -> stringResource(R.string.task_toast_failedUpdate)
    }
