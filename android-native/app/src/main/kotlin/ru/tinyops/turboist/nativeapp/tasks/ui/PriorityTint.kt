package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import ru.tinyops.turboist.core.model.Priority

/**
 * The colour a priority is signalled in.
 *
 * Taken from the active colour scheme rather than from fixed values, so the
 * three levels stay legible in a light scheme, in a dark one, and under the
 * wallpaper-derived palette the system may hand us. Urgency reads as the
 * scheme's alarm colour, the middle level as its second accent, and the lowest
 * as its main one; a task with no priority set is not signalled at all and takes
 * the outline colour a plain control would.
 *
 * A priority spelling this build does not recognise is drawn as unset. It is the
 * only honest option: the row must appear, and inventing a level for it would
 * misreport it.
 */
@Composable
@ReadOnlyComposable
fun priorityTint(priority: Priority): Color =
    when (priority) {
        Priority.HIGH -> MaterialTheme.colorScheme.error
        Priority.MEDIUM -> MaterialTheme.colorScheme.tertiary
        Priority.LOW -> MaterialTheme.colorScheme.primary
        Priority.NONE, Priority.UNKNOWN -> MaterialTheme.colorScheme.outline
    }
