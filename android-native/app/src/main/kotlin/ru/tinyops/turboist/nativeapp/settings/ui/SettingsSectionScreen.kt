package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import ru.tinyops.turboist.core.model.MAX_MAX_PINNED
import ru.tinyops.turboist.core.model.MIN_MAX_PINNED
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.DeletedRule
import ru.tinyops.turboist.nativeapp.settings.SettingsMessage
import ru.tinyops.turboist.nativeapp.settings.SettingsSection
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.settings.SettingsViewModel

/** One section of settings as the app builds it. */
@Composable
fun SettingsSectionScreen(
    section: SettingsSection,
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    SettingsSectionScreen(
        section = section,
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
                onRestoreRule = presenter::restoreRule,
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
 * One section of settings: its groups, and the sheet and questions its rows open.
 *
 * Every section sits over the same three stores, and each group in it says which
 * one when that is not obvious — the theme beside the language is the case that
 * needs it most.
 */
@Composable
fun SettingsSectionScreen(
    section: SettingsSection,
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    messages: Flow<SettingsMessage> = emptyFlow(),
) {
    val snackbars = remember { SnackbarHostState() }
    SettingsMessageHost(messages, snackbars, callbacks.onRestoreRule)

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
            when (section) {
                SettingsSection.GENERAL -> GeneralSection(state, callbacks)
                SettingsSection.WORKSPACE -> WorkspaceSection(state, callbacks)
                SettingsSection.RULES -> RulesSection(state, callbacks)
                SettingsSection.SECURITY -> SettingsSectionLinks(section, destinations)
                SettingsSection.DEVICE -> DeviceSection(state, callbacks, destinations)
                SettingsSection.ABOUT -> AboutSection(state)
            }
            SettingsBottomSpace()
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter).padding(8.dp))
    }

    state.editor?.let { editor ->
        RuleEditorSheet(editor = editor, state = state, callbacks = callbacks)
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
 * The rows of a section that lead to other screens.
 *
 * Drawn apart from the controls beside them so the navigation walk can show the
 * real rows without the stores the controls read. The security section is
 * nothing but these: every screen it leads to talks to the server.
 */
@Composable
fun SettingsSectionLinks(
    section: SettingsSection,
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    refusedChangeCount: Int = 0,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        when (section) {
            SettingsSection.SECURITY -> SecurityLinks(destinations)
            SettingsSection.DEVICE ->
                SettingsGroup { row { UnsentChangesRow(refusedChangeCount, destinations.openUnsentChanges) } }

            else -> Unit
        }
    }
}

/**
 * The way to the pile of changes the server refused. It lives in settings
 * rather than in the drawer because it is a place the user goes when something
 * is wrong; the count beside it is what makes it findable without a permanent
 * entry shouting about an empty list.
 */
@Composable
internal fun UnsentChangesRow(
    refusedChangeCount: Int,
    onOpen: () -> Unit,
) {
    SettingsNavRow(
        title = stringResource(R.string.offline_unsentTitle),
        summary = stringResource(R.string.native_unsent_settingsDescription),
        icon = Icons.Outlined.CloudOff,
        tintedIcon = false,
        badge = refusedChangeCount,
        onClick = onOpen,
    )
}

@Composable
private fun SecurityLinks(destinations: SettingsDestinations) {
    SettingsGroupLabel(stringResource(R.string.native_settings_signInHeading))
    SettingsGroup {
        row {
            SettingsNavRow(
                title = stringResource(R.string.settings_passkeys_heading),
                summary = stringResource(R.string.settings_passkeys_description),
                icon = Icons.Outlined.Key,
                onClick = destinations.openPasskeys,
            )
        }
        row {
            SettingsNavRow(
                title = stringResource(R.string.settings_twofa_heading),
                summary = stringResource(R.string.settings_twofa_description),
                icon = Icons.Outlined.Shield,
                onClick = destinations.openTwoFactor,
            )
        }
    }
    // None of the screens below is replicated, so each one needs a connection to
    // say anything at all — which is why they are rows leading away rather than
    // controls sitting in a screen that works offline.
    SettingsGroupLabel(stringResource(R.string.native_settings_accessHeading))
    SettingsGroup {
        row {
            SettingsNavRow(
                title = stringResource(R.string.settings_sessions_heading),
                summary = stringResource(R.string.settings_sessions_description),
                icon = Icons.Outlined.Devices,
                onClick = destinations.openSessions,
            )
        }
        row {
            SettingsNavRow(
                title = stringResource(R.string.settings_api_heading),
                summary = stringResource(R.string.settings_api_description),
                icon = Icons.Outlined.Terminal,
                onClick = destinations.openApiTokens,
            )
        }
    }
    SettingsNote(
        text = stringResource(R.string.native_settings_securityOnlineNote),
        icon = Icons.Outlined.CloudOff,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/**
 * Shows what the presenter has to say, one sentence at a time.
 *
 * A deleted rule is the one message with an action: the deletion has already
 * been written, so taking it back is a second write rather than a cancelled one.
 */
@Composable
private fun SettingsMessageHost(
    messages: Flow<SettingsMessage>,
    snackbars: SnackbarHostState,
    onRestoreRule: (DeletedRule) -> Unit,
) {
    var pending by remember { mutableStateOf<SettingsMessage?>(null) }
    LaunchedEffect(messages) { messages.collect { pending = it } }
    val message = pending ?: return
    val text = settingsMessageText(message)
    val undo = stringResource(R.string.native_settings_ruleUndo)
    LaunchedEffect(message, text) {
        if (message is SettingsMessage.RuleDeleted) {
            val result = snackbars.showSnackbar(text, actionLabel = undo, withDismissAction = true)
            if (result == SnackbarResult.ActionPerformed) onRestoreRule(message.deleted)
        } else {
            snackbars.showSnackbar(text)
        }
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
        is SettingsMessage.RuleDeleted ->
            stringResource(
                R.string.native_settings_ruleDeleted,
                when (val deleted = message.deleted) {
                    is DeletedRule.AutoLabel -> deleted.rule.mask
                    is DeletedRule.ProjectSuggestion -> deleted.rule.mask
                },
            )
    }
