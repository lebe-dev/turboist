package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.Bookmark
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Dashboard
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.DeletedRule
import ru.tinyops.turboist.nativeapp.settings.RuleKind
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsSearchEntry
import ru.tinyops.turboist.nativeapp.settings.SettingsSection
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.settings.SettingsViewModel
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import ru.tinyops.turboist.nativeapp.settings.matchSettings
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/** Everything a settings section can be asked to do. */
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
    val onRestoreRule: (DeletedRule) -> Unit = {},
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
 * Where each of the rows of settings and its sections leads.
 *
 * Grouped into one value rather than passed one lambda at a time because the
 * group only grows: settings is the entrance to every surface that is a detail
 * of the account or of the app rather than a place work happens, and a screen
 * that took each of those as its own parameter would be re-signed every time one
 * was added.
 *
 * The graph fills this in; the screens name no route.
 */
data class SettingsDestinations(
    val openSection: (SettingsSection) -> Unit = {},
    val openPasskeys: () -> Unit = {},
    val openSessions: () -> Unit = {},
    val openApiTokens: () -> Unit = {},
    val openTwoFactor: () -> Unit = {},
    val openTemplates: () -> Unit = {},
    val openUnsentChanges: () -> Unit = {},
)

/** The name of a section, as its row and the bar over it say it. */
val SettingsSection.titleRes: Int
    get() =
        when (this) {
            SettingsSection.GENERAL -> R.string.native_settings_section_general
            SettingsSection.WORKSPACE -> R.string.native_settings_section_workspace
            SettingsSection.RULES -> R.string.native_settings_section_rules
            SettingsSection.SECURITY -> R.string.native_settings_section_security
            SettingsSection.DEVICE -> R.string.native_settings_deviceHeading
            SettingsSection.ABOUT -> R.string.native_settings_aboutHeading
        }

private val SettingsSection.icon: ImageVector
    get() =
        when (this) {
            SettingsSection.GENERAL -> Icons.Outlined.Tune
            SettingsSection.WORKSPACE -> Icons.Outlined.Dashboard
            SettingsSection.RULES -> Icons.Outlined.Bolt
            SettingsSection.SECURITY -> Icons.Outlined.Shield
            SettingsSection.DEVICE -> Icons.Outlined.PhoneAndroid
            SettingsSection.ABOUT -> Icons.Outlined.Info
        }

/** The top level as the app builds it. */
@Composable
fun SettingsScreen(
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.presenter.state.collectAsStateWithLifecycle()
    SettingsScreen(state = state, destinations = destinations, modifier = modifier)
}

/**
 * The top level of settings: a map of the sections, and a way to find one
 * setting without knowing which section holds it.
 *
 * No control sits here. A switch on this screen would be one more thing among
 * the sections rather than in one of them, and the screen that was a long
 * list of everything is exactly what this replaces. What does sit here is the
 * one thing that needs the user's attention before any choice does: changes the
 * server refused.
 */
@Composable
fun SettingsScreen(
    state: SettingsUiState,
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
) {
    var query by rememberSaveable { mutableStateOf("") }
    val entries = settingsSearchEntries()
    val hits = matchSettings(query, entries)

    Column(modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
        SettingsSearchField(query = query, onQueryChange = { query = it })
        if (query.isNotBlank()) {
            SettingsSearchResults(hits = hits, onOpen = destinations.openSection)
        } else {
            if (state.refusedChangeCount > 0) {
                RefusedChangesCard(count = state.refusedChangeCount, onOpen = destinations.openUnsentChanges)
            }
            SettingsNavigationRows(
                destinations = destinations,
                generalSummary = generalSummary(state),
                aboutSummary =
                    stringResource(
                        R.string.native_settings_section_aboutSummary,
                        state.version.ifEmpty { stringResource(R.string.native_settings_versionUnknown) },
                    ),
                refusedChangeCount = state.refusedChangeCount,
            )
        }
        SettingsBottomSpace()
    }
}

/**
 * The rows of the top level, grouped by what the user is there to do.
 *
 * Stateless and handed its summaries, so the navigation walk can draw the real
 * rows without the stores behind them. The graph, not this screen, decides
 * where a row leads.
 */
@Composable
fun SettingsNavigationRows(
    destinations: SettingsDestinations,
    modifier: Modifier = Modifier,
    generalSummary: String? = null,
    aboutSummary: String? = null,
    refusedChangeCount: Int = 0,
) {
    val sectionRow: @Composable (SettingsSection, String?, Boolean, Int?) -> Unit = { section, summary, tinted, badge ->
        SettingsNavRow(
            title = stringResource(section.titleRes),
            summary = summary,
            icon = section.icon,
            tintedIcon = tinted,
            badge = badge,
            onClick = { destinations.openSection(section) },
        )
    }
    Column(modifier = modifier.fillMaxWidth()) {
        SettingsGroupLabel(stringResource(R.string.native_settings_group_work))
        SettingsGroup {
            row { sectionRow(SettingsSection.GENERAL, generalSummary, true, null) }
            row {
                sectionRow(
                    SettingsSection.WORKSPACE,
                    stringResource(R.string.native_settings_section_workspaceSummary),
                    true,
                    null,
                )
            }
            row {
                sectionRow(
                    SettingsSection.RULES,
                    stringResource(R.string.native_settings_section_rulesSummary),
                    true,
                    null,
                )
            }
            row {
                SettingsNavRow(
                    title = stringResource(R.string.settings_templates_heading),
                    summary = stringResource(R.string.native_settings_templatesSummary),
                    icon = Icons.Outlined.Bookmark,
                    onClick = destinations.openTemplates,
                )
            }
        }

        SettingsGroupLabel(stringResource(R.string.native_settings_group_account))
        SettingsGroup {
            row {
                sectionRow(
                    SettingsSection.SECURITY,
                    stringResource(R.string.native_settings_section_securitySummary),
                    true,
                    null,
                )
            }
        }

        SettingsGroupLabel(stringResource(R.string.native_settings_group_system))
        SettingsGroup {
            row {
                sectionRow(
                    SettingsSection.DEVICE,
                    stringResource(R.string.native_settings_section_deviceSummary),
                    false,
                    refusedChangeCount,
                )
            }
            row { sectionRow(SettingsSection.ABOUT, aboutSummary, false, null) }
        }
    }
}

/**
 * The pile of changes the server refused, announced above everything else.
 *
 * It is the one thing on settings that is not a choice: work the user did is
 * sitting on this phone, and nothing but a person can send it again or let it go.
 */
@Composable
private fun RefusedChangesCard(
    count: Int,
    onOpen: () -> Unit,
) {
    val scheme = MaterialTheme.colorScheme
    Surface(
        shape = RoundedCornerShape(20.dp),
        color = scheme.errorContainer,
        contentColor = scheme.onErrorContainer,
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 12.dp),
    ) {
        Row(
            modifier = Modifier.clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Icon(imageVector = Icons.Outlined.CloudOff, contentDescription = null)
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = pluralStringResource(R.plurals.native_settings_refusedBanner, count, count),
                    style = MaterialTheme.typography.titleSmall,
                )
                Text(
                    text = stringResource(R.string.native_settings_refusedBannerBody),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Icon(imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null)
        }
    }
}

@Composable
private fun SettingsSearchField(
    query: String,
    onQueryChange: (String) -> Unit,
) {
    TextField(
        value = query,
        onValueChange = onQueryChange,
        singleLine = true,
        placeholder = { Text(stringResource(R.string.native_settings_searchHint)) },
        leadingIcon = { Icon(imageVector = Icons.Outlined.Search, contentDescription = null) },
        trailingIcon = {
            if (query.isNotEmpty()) {
                IconButton(onClick = { onQueryChange("") }) {
                    Icon(
                        imageVector = Icons.Outlined.Close,
                        contentDescription = stringResource(R.string.native_settings_searchClear),
                    )
                }
            }
        },
        shape = RoundedCornerShape(28.dp),
        colors =
            TextFieldDefaults.colors(
                focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
            ),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    )
}

@Composable
private fun SettingsSearchResults(
    hits: List<SettingsSearchEntry>,
    onOpen: (SettingsSection) -> Unit,
) {
    if (hits.isEmpty()) {
        Text(
            text = stringResource(R.string.native_settings_searchEmpty),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 32.dp, vertical = 24.dp),
        )
        return
    }
    SettingsGroup(modifier = Modifier.padding(top = 16.dp)) {
        for (hit in hits) {
            row {
                ListItem(
                    headlineContent = { Text(hit.title) },
                    supportingContent = { Text(hit.sectionTitle) },
                    trailingContent = {
                        Icon(imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null)
                    },
                    colors = groupRowColors(),
                    modifier = Modifier.clickable { onOpen(hit.section) },
                )
            }
        }
    }
}

/**
 * Every setting a search can find, in words.
 *
 * Each one is matched on its name, its section's name and the sentence that
 * describes it, so a word from the description — "dark" for the theme — finds
 * it as well as its name does.
 */
@Composable
private fun settingsSearchEntries(): List<SettingsSearchEntry> {
    val catalog: List<Triple<Int, SettingsSection, Int?>> =
        listOf(
            Triple(R.string.settings_language_heading, SettingsSection.GENERAL, R.string.settings_language_description),
            Triple(R.string.settings_theme_heading, SettingsSection.GENERAL, R.string.settings_theme_description),
            Triple(R.string.settings_privacy_heading, SettingsSection.GENERAL, R.string.settings_privacy_hint),
            Triple(R.string.settings_banner_heading, SettingsSection.WORKSPACE, R.string.settings_banner_description),
            Triple(
                R.string.settings_menu_pinned_heading,
                SettingsSection.WORKSPACE,
                R.string.settings_menu_pinned_description,
            ),
            Triple(R.string.settings_troiki_heading, SettingsSection.WORKSPACE, R.string.settings_troiki_description),
            Triple(R.string.settings_calendars_heading, SettingsSection.WORKSPACE, null),
            Triple(
                R.string.settings_autoLabels_heading,
                SettingsSection.RULES,
                R.string.settings_autoLabels_description,
            ),
            Triple(R.string.settings_projectSuggestions_heading, SettingsSection.RULES, null),
            Triple(R.string.settings_weekly_heading, SettingsSection.RULES, R.string.settings_weekly_description),
            Triple(
                R.string.settings_project_bugLabelsHeading,
                SettingsSection.RULES,
                R.string.settings_project_bugLabelsDescription,
            ),
            Triple(
                R.string.settings_passkeys_heading,
                SettingsSection.SECURITY,
                R.string.settings_passkeys_description,
            ),
            Triple(R.string.settings_twofa_heading, SettingsSection.SECURITY, R.string.settings_twofa_description),
            Triple(
                R.string.settings_sessions_heading,
                SettingsSection.SECURITY,
                R.string.settings_sessions_description,
            ),
            Triple(R.string.settings_api_heading, SettingsSection.SECURITY, R.string.settings_api_description),
            Triple(
                R.string.native_settings_syncOnMetered,
                SettingsSection.DEVICE,
                R.string.native_settings_syncOnMeteredDescription,
            ),
            Triple(R.string.offline_unsentTitle, SettingsSection.DEVICE, R.string.native_unsent_settingsDescription),
            Triple(R.string.native_settings_serverHeading, SettingsSection.DEVICE, null),
            Triple(
                R.string.native_settings_changeServer,
                SettingsSection.DEVICE,
                R.string.native_settings_changeServerDescription,
            ),
            Triple(
                R.string.native_settings_clearLocalData,
                SettingsSection.DEVICE,
                R.string.native_settings_clearLocalDataDescription,
            ),
            Triple(R.string.settings_version_heading, SettingsSection.ABOUT, null),
            Triple(R.string.legal_tos_title, SettingsSection.ABOUT, null),
            Triple(R.string.legal_privacy_title, SettingsSection.ABOUT, null),
        )
    return catalog.map { (titleRes, section, descriptionRes) ->
        SettingsSearchEntry(
            title = stringResource(titleRes),
            section = section,
            sectionTitle = stringResource(section.titleRes),
            keywords = descriptionRes?.let { stringResource(it) }.orEmpty(),
        )
    }
}

/** What the general section holds right now, so the row says it without being opened. */
@Composable
private fun generalSummary(state: SettingsUiState): String {
    val language = OFFERED_LOCALES.firstOrNull { it.first == state.user.locale }?.second ?: state.user.locale
    val theme = stringResource(THEME_CHOICES.first { it.first == state.device.theme }.second)
    return stringResource(R.string.native_settings_section_generalSummary, language, theme)
}

@Preview(showBackground = true)
@Composable
private fun SettingsScreenPreview() {
    TurboistTheme {
        SettingsScreen(
            state = SettingsUiState(loading = false, version = "1.2.3", refusedChangeCount = 3),
            destinations = SettingsDestinations(),
        )
    }
}
