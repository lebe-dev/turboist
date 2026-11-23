package ru.tinyops.turboist.nativeapp.ui

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * A message over a screen whose bottom corner holds an add button.
 *
 * A snackbar is drawn along the bottom edge, which is exactly where that button
 * stands. Left there, a confirmation covers the one control the next action
 * starts from — and it does so right after telling the user the last one worked,
 * so the very thing they want to repeat is the thing hidden. The message takes
 * the space above the button instead, as a scaffold places it, and the two never
 * overlap.
 */
@Composable
fun SnackbarAboveFab(
    hostState: SnackbarHostState,
    modifier: Modifier = Modifier,
    fabShown: Boolean = true,
) {
    SnackbarHost(
        hostState = hostState,
        modifier = modifier.padding(bottom = if (fabShown) FAB_CLEARANCE else 0.dp),
    )
}

/**
 * What a standard button takes from the bottom edge: the 16dp it keeps clear,
 * its own 56dp, and a little air so a message never touches it.
 */
private val FAB_CLEARANCE = 16.dp + 56.dp + 8.dp
