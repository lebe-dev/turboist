package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The colour a priority is signalled in.
 *
 * The three levels are the web client's own red, amber and blue rather than
 * roles borrowed from the colour scheme: a user reads "red" as urgent across
 * both front ends, and a level drawn in whatever the scheme happens to call its
 * second accent would say something different on each. A task with no priority
 * is not signalled at all and takes the outline colour a plain control would.
 *
 * A priority spelling this build does not recognise is drawn as unset. It is the
 * only honest option: the row must appear, and inventing a level for it would
 * misreport it.
 */
@Composable
@ReadOnlyComposable
fun priorityTint(priority: Priority): Color =
    when (priority) {
        Priority.HIGH -> TurboistTheme.accents.priorityHigh
        Priority.MEDIUM -> TurboistTheme.accents.priorityMedium
        Priority.LOW -> TurboistTheme.accents.priorityLow
        Priority.NONE, Priority.UNKNOWN -> MaterialTheme.colorScheme.outline
    }
