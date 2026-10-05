package ru.tinyops.turboist.nativeapp.shell

import android.content.Intent
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.navigation.AuthNavHost
import ru.tinyops.turboist.nativeapp.session.ShellGraph
import ru.tinyops.turboist.nativeapp.session.graphFor
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The whole content tree.
 *
 * The one decision made here is which of the three top-level trees to show, and
 * it is made from session state alone. Keeping the authentication screens and
 * the app in separate graphs means a signed-out user has no back stack into the
 * app, and signing out cannot leave a half-torn-down task screen behind.
 */
@Composable
fun TurboistRoot(
    newIntents: Flow<Intent> = emptyFlow(),
    modifier: Modifier = Modifier,
    viewModel: AppViewModel = hiltViewModel(),
) {
    val session by viewModel.sessionState.collectAsStateWithLifecycle()
    val counts by viewModel.drawerCounts.collectAsStateWithLifecycle()
    val authStep by viewModel.authStep.collectAsStateWithLifecycle()
    val unverified by viewModel.unverifiedSession.collectAsStateWithLifecycle()
    val prompt by viewModel.unsentChangesPrompt.collectAsStateWithLifecycle()
    val syncStatus by viewModel.syncStatus.collectAsStateWithLifecycle()
    val theme by viewModel.theme.collectAsStateWithLifecycle()
    val dailyPlanEnabled by viewModel.dailyPlanEnabled.collectAsStateWithLifecycle()

    TurboistTheme(darkTheme = darkFor(theme)) {
        when (graphFor(session)) {
            ShellGraph.Splash -> SplashScreen(modifier = modifier)
            ShellGraph.Auth ->
                AuthNavHost(
                    navController = rememberNavController(),
                    startStep = authStep,
                    modifier = modifier,
                )

            ShellGraph.App ->
                AppShell(
                    counts = counts,
                    newIntents = newIntents,
                    unverifiedSession = unverified,
                    onSignOut = viewModel::signOut,
                    modifier = modifier,
                    syncStatus = syncStatus,
                    onRetrySync = viewModel::retrySync,
                    dailyPlanEnabled = dailyPlanEnabled,
                    pendingTaskLinks = viewModel.pendingTaskLinks,
                )
        }

        prompt?.let { UnsentChangesDialog(it) }
    }
}

/**
 * Whether to draw dark, given what this device was told to do.
 *
 * Following the phone is the default rather than a third palette: an app that
 * ignored the system setting would be the only one on the device that did. The
 * two explicit answers exist because a phone with no system-wide switch, or a
 * user who wants this one app to differ, has nowhere else to say so.
 */
@Composable
private fun darkFor(theme: ThemeChoice): Boolean =
    when (theme) {
        ThemeChoice.SYSTEM -> isSystemInDarkTheme()
        ThemeChoice.LIGHT -> false
        ThemeChoice.DARK -> true
    }

/**
 * The last chance to keep changes the server has never seen.
 *
 * Signing out empties the device, so anything still queued goes with it. The
 * count is shown because "some changes" is not something a user can weigh, and
 * declining leaves both the session and the queue exactly as they were.
 */
@Composable
private fun UnsentChangesDialog(prompt: UnsentChangesPrompt) {
    AlertDialog(
        onDismissRequest = prompt::keep,
        title = { Text(stringResource(R.string.offline_unsentTitle)) },
        text = { Text(stringResource(R.string.offline_unsentBody, prompt.count)) },
        confirmButton = {
            TextButton(onClick = prompt::discard) {
                Text(stringResource(R.string.offline_unsentDiscard))
            }
        },
        dismissButton = {
            TextButton(onClick = prompt::keep) {
                Text(stringResource(R.string.common_cancel))
            }
        },
    )
}
