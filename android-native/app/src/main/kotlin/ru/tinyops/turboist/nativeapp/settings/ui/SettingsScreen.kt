package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Bookmark
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.MAX_MAX_PINNED
import ru.tinyops.turboist.core.model.MIN_MAX_PINNED
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.RuleKind
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsMessage
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.settings.SettingsViewModel
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/** Everything the settings screen can be asked to do. */
data class SettingsCallbacks(
    val onSetLocale: (String) -> Unit = {},
    val onSetPublicView: (Boolean) -> Unit = {},
    val onSetTroikiEnabled: (Boolean) -> Unit = {},
    val onSetCalendarEnabled: (Boolean) -> Unit = {},
    val onSetCalendarHidePastEvents: (Boolean) -> Unit = {},
    val onEditBannerText: (String) -> Unit = {},
    val onSaveBannerText: () -> Unit = {},
    val onSetBannerPublished: (Boolean) -> Unit = {},
    val onSetBannerDayPart: (DayPart?) -> Unit = {},
    val onEditMaxPinnedTasks: (String) -> Unit = {},
    val onEditMaxPinnedProjects: (String) -> Unit = {},
    val onSavePinnedCaps: () -> Unit = {},
    val onToggleWeeklyExcludedLabel: (Long) -> Unit = {},
    val onToggleBugLabel: (Long) -> Unit = {},
    val onStartNewRule: (RuleKind) -> Unit = {},
    val onEditRule: (RuleKind, Int) -> Unit = { _, _ -> },
    val onDeleteRule: (RuleKind, Int) -> Unit = { _, _ -> },
    val onSetRuleMask: (String) -> Unit = {},
    val onSetRuleIgnoreCase: (Boolean) -> Unit = {},
    val onToggleRuleTarget: (Long) -> Unit = {},
    val onSaveRule: () -> Unit = {},
    val onCancelRule: () -> Unit = {},
    val onSetTheme: (ThemeChoice) -> Unit = {},
    val onSetSyncOnMetered: (Boolean) -> Unit = {},
    val onAsk: (SettingsConfirmation) -> Unit = {},
    val onCancelConfirmation: () -> Unit = {},
    val onConfirm: () -> Unit = {},
)

/**
 * Where each of the settings screen's navigation rows leads.
 *
 * Grouped into one value rather than passed one lambda at a time because the
 * group only grows: settings is the entrance to every surface that is a detail
 * of the account or of the app rather than a place work happens, and a screen
 * that took each of those as its own parameter would be re-signed every time one
 * was added.
 *
 * The graph fills this in; the screen names no route.
 */
data class SettingsDestinations(
    val openPasskeys: () -> Unit = {},
    val openSessions: () -> Unit = {},
    val openApiTokens: () -> Unit = {},
    val openTwoFactor: () -> Unit = {},
    val openTemplates: () -> Unit = {},
    val openUnsentChanges: () -> Unit = {},
)

/** The screen as the app builds it. */
@Composable
fun SettingsScreen(
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    SettingsScreen(
        state = state,
        messages = presenter.messages,
        callbacks =
            SettingsCallbacks(
                onSetLocale = presenter::setLocale,
                onSetPublicView = presenter::setPublicView,
                onSetTroikiEnabled = presenter::setTroikiEnabled,
                onSetCalendarEnabled = presenter::setCalendarEnabled,
                onSetCalendarHidePastEvents = presenter::setCalendarHidePastEvents,
                onEditBannerText = presenter::editBannerText,
                onSaveBannerText = presenter::saveBannerText,
                onSetBannerPublished = presenter::setBannerPublished,
                onSetBannerDayPart = presenter::setBannerDayPart,
                onEditMaxPinnedTasks = presenter::editMaxPinnedTasks,
                onEditMaxPinnedProjects = presenter::editMaxPinnedProjects,
                onSavePinnedCaps = presenter::savePinnedCaps,
                onToggleWeeklyExcludedLabel = presenter::toggleWeeklyExcludedLabel,
                onToggleBugLabel = presenter::toggleBugLabel,
                onStartNewRule = presenter::startNewRule,
                onEditRule = presenter::editRule,
                onDeleteRule = presenter::deleteRule,
                onSetRuleMask = presenter::setRuleMask,
                onSetRuleIgnoreCase = presenter::setRuleIgnoreCase,
                onToggleRuleTarget = presenter::toggleRuleTarget,
                onSaveRule = presenter::saveRule,
                onCancelRule = presenter::cancelRule,
                onSetTheme = presenter::setTheme,
                onSetSyncOnMetered = presenter::setSyncOnMetered,
                onAsk = presenter::ask,
                onCancelConfirmation = presenter::cancelConfirmation,
                onConfirm = presenter::confirm,
            ),
        destinations = destinations,
        modifier = modifier,
    )
}

/**
 * One settings surface over three stores that must never be confused.
 *
 * The order is deliberate and is the whole navigation aid this screen has: the
 * account's own preferences first, then the rules the whole installation obeys,
 * then the choices that never leave this phone, then what the build is. Each
 * group says which of the three it belongs to, because the same-looking switch
 * means very different things depending on which document it writes to.
 *
 * The rows that lead somewhere else sit at the top, above the controls, because
 * they are the way to the screens settings is the only entrance to.
 */
@Composable
fun SettingsScreen(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    messages: Flow<SettingsMessage> = emptyFlow(),
) {
    val snackbars = remember { SnackbarHostState() }
    SettingsMessageHost(messages, snackbars)

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
            SettingsNavigationRows(destinations)
            UserPreferencesSection(state, callbacks)
            HorizontalDivider()
            AppRulesSection(state, callbacks)
            HorizontalDivider()
            DeviceSection(state, callbacks)
            HorizontalDivider()
            AboutSection(state)
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter).padding(8.dp))
    }

    state.editor?.let { editor ->
        RuleEditorDialog(editor = editor, state = state, callbacks = callbacks)
    }
    state.confirming?.let { pending ->
        DestructiveConfirmationDialog(
            confirmation = pending,
            unsentChangeCount = state.unsentChangeCount,
            onConfirm = callbacks.onConfirm,
            onDismiss = callbacks.onCancelConfirmation,
        )
    }
}

/**
 * The rows that lead to the screens settings is the only entrance to.
 *
 * They are here rather than in the drawer because none of them is a place work
 * happens: how the account signs in, the blueprints work is written from, and
 * the pile the server refused. The graph, not this screen, decides where a row
 * leads, which is why each one is handed in as a callback.
 */
@Composable
fun SettingsNavigationRows(
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
) {
    // Stacked here rather than by whoever draws them: they are a group of rows
    // and each one has to be its own tap target, which is not true of three
    // list items dropped into a container that lays its children on top of each
    // other.
    Column(modifier = modifier.fillMaxWidth()) {
        SettingsRow(
            titleRes = R.string.settings_passkeys_heading,
            descriptionRes = R.string.settings_passkeys_description,
            icon = Icons.Outlined.Key,
            onClick = destinations.openPasskeys,
        )
        HorizontalDivider()
        // The three administrative surfaces. None of them is replicated, so each
        // one needs a connection to say anything at all — which is why they are
        // rows leading away rather than controls sitting in a screen that works
        // offline.
        SettingsRow(
            titleRes = R.string.settings_sessions_heading,
            descriptionRes = R.string.settings_sessions_description,
            icon = Icons.Outlined.Devices,
            onClick = destinations.openSessions,
        )
        HorizontalDivider()
        SettingsRow(
            titleRes = R.string.settings_twofa_heading,
            descriptionRes = R.string.settings_twofa_description,
            icon = Icons.Outlined.Shield,
            onClick = destinations.openTwoFactor,
        )
        HorizontalDivider()
        SettingsRow(
            titleRes = R.string.settings_api_heading,
            descriptionRes = R.string.settings_api_description,
            icon = Icons.Outlined.Terminal,
            onClick = destinations.openApiTokens,
        )
        HorizontalDivider()
        SettingsRow(
            titleRes = R.string.settings_templates_heading,
            descriptionRes = R.string.settings_templates_description,
            icon = Icons.Outlined.Bookmark,
            onClick = destinations.openTemplates,
        )
        HorizontalDivider()
        // The way to the pile of changes the server refused. It lives in settings
        // rather than in the drawer because it is a place the user goes when
        // something is wrong; the drawer entry that leads here carries a count when
        // the pile is not empty, which is what makes it findable without a permanent
        // row shouting about an empty list.
        SettingsRow(
            titleRes = R.string.offline_unsentTitle,
            descriptionRes = R.string.native_unsent_settingsDescription,
            icon = Icons.Outlined.CloudOff,
            onClick = destinations.openUnsentChanges,
        )
    }
}

@Composable
private fun SettingsRow(
    titleRes: Int,
    descriptionRes: Int,
    icon: ImageVector,
    onClick: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(stringResource(titleRes)) },
        supportingContent = { Text(stringResource(descriptionRes)) },
        leadingContent = { Icon(imageVector = icon, contentDescription = null) },
        // The chevron says the row leads somewhere rather than toggling
        // something; it carries no description of its own because the row's own
        // wording is what a screen reader should announce.
        trailingContent = {
            Icon(imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null)
        },
        modifier = Modifier.clickable(onClick = onClick),
    )
}

/** Shows what the presenter has to say, one sentence at a time. */
@Composable
private fun SettingsMessageHost(
    messages: Flow<SettingsMessage>,
    snackbars: SnackbarHostState,
) {
    var pending by remember { mutableStateOf<SettingsMessage?>(null) }
    LaunchedEffect(messages) { messages.collect { pending = it } }
    val message = pending ?: return
    val text = settingsMessageText(message)
    LaunchedEffect(message, text) {
        snackbars.showSnackbar(text)
        pending = null
    }
}

@Composable
private fun settingsMessageText(message: SettingsMessage): String =
    when (message) {
        SettingsMessage.Saved -> stringResource(R.string.native_settings_saved)
        SettingsMessage.SaveFailed -> stringResource(R.string.native_settings_saveFailed)
        SettingsMessage.PinnedCapOutOfRange ->
            stringResource(
                R.string.settings_menu_pinned_toastRange,
                MIN_MAX_PINNED.toString(),
                MAX_MAX_PINNED.toString(),
            )

        SettingsMessage.RuleIncomplete -> stringResource(R.string.native_settings_ruleMaskEmpty)
        SettingsMessage.LocalDataCleared -> stringResource(R.string.native_settings_localDataCleared)
    }

@Preview(showBackground = true)
@Composable
private fun SettingsScreenPreview() {
    TurboistTheme {
        SettingsScreen(
            state = SettingsUiState(loading = false, serverAddress = "https://turboist.example/", version = "1.2.3"),
            callbacks = SettingsCallbacks(),
            destinations = SettingsDestinations(),
        )
    }
}
