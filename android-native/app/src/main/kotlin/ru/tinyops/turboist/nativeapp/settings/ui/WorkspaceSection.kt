package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Campaign
import androidx.compose.material.icons.outlined.Remove
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.MAX_MAX_PINNED
import ru.tinyops.turboist.core.model.MIN_MAX_PINNED
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState

/** The four states of the banner's day-phase gate; the absent one means all day. */
private val BANNER_DAY_PARTS: List<Pair<DayPart?, Int>> =
    listOf(
        null to R.string.settings_banner_dayPart_allDay,
        DayPart.MORNING to R.string.settings_banner_dayPart_morning,
        DayPart.AFTERNOON to R.string.settings_banner_dayPart_afternoon,
        DayPart.EVENING to R.string.settings_banner_dayPart_evening,
    )

/** A markdown link, which the preview shows as its words alone. */
private val MarkdownLink = Regex("""\[([^\]]+)]\([^)]+\)""")

/**
 * How the places work happens in behave: the banner over Today, the shelf of
 * pinned things in the drawer, the daily plan, and calendars.
 *
 * All of it belongs to the account and follows the user to every device.
 */
@Composable
internal fun WorkspaceSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    BannerSettings(state, callbacks)
    PinnedCapSettings(state, callbacks)

    SettingsGroupLabel(stringResource(R.string.settings_troiki_heading))
    SettingsGroup {
        row {
            SettingsSwitchRow(
                title = stringResource(R.string.settings_troiki_toggle),
                checked = state.user.troikiEnabled,
                onCheckedChange = callbacks.onSetTroikiEnabled,
                description = stringResource(R.string.settings_troiki_description),
            )
        }
    }

    SettingsGroupLabel(stringResource(R.string.settings_calendars_heading))
    SettingsGroup {
        row {
            SettingsSwitchRow(
                titleRes = R.string.settings_calendars_enableLabel,
                checked = state.user.calendarEnabled,
                onCheckedChange = callbacks.onSetCalendarEnabled,
            )
        }
        row {
            // Means nothing while there are no calendars on screen, so it waits
            // for them rather than taking a choice that would change nothing.
            SettingsSwitchRow(
                titleRes = R.string.settings_calendars_hidePastEvents,
                checked = state.user.calendarHidePastEvents,
                onCheckedChange = callbacks.onSetCalendarHidePastEvents,
                descriptionRes = R.string.settings_calendars_hidePastEventsDescription,
                enabled = state.user.calendarEnabled,
            )
        }
        row { CalendarSettingsNote() }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun BannerSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsGroupLabel(stringResource(R.string.settings_banner_heading))
    SettingsGroup {
        row {
            Column(
                modifier = Modifier.fillMaxWidth().padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                BannerPreview(state.bannerText)
                OutlinedTextField(
                    value = state.bannerText,
                    onValueChange = callbacks.onEditBannerText,
                    label = { Text(stringResource(R.string.settings_banner_textLabel)) },
                    placeholder = { Text(stringResource(R.string.settings_banner_textPlaceholder)) },
                    supportingText = { Text(stringResource(R.string.settings_banner_description)) },
                    modifier = Modifier.fillMaxWidth(),
                )
                // The text is the one preference here with a save button, because
                // it is typed rather than chosen: writing on every keystroke would
                // queue a request per character and hand the server a half-written
                // sentence. The button appears only when there is something to save.
                AnimatedVisibility(visible = state.bannerEdited, modifier = Modifier.align(Alignment.End)) {
                    Button(onClick = callbacks.onSaveBannerText) { Text(stringResource(R.string.common_save)) }
                }
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = stringResource(R.string.settings_banner_dayPart_label),
                        style = MaterialTheme.typography.labelLarge,
                    )
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        for ((dayPart, wording) in BANNER_DAY_PARTS) {
                            FilterChip(
                                selected = state.user.bannerDayPart == dayPart,
                                onClick = { callbacks.onSetBannerDayPart(dayPart) },
                                label = { Text(stringResource(wording)) },
                            )
                        }
                    }
                    Text(
                        text = stringResource(R.string.settings_banner_dayPart_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        row {
            SettingsSwitchRow(
                titleRes = R.string.settings_banner_publishLabel,
                checked = state.user.bannerPublished,
                onCheckedChange = callbacks.onSetBannerPublished,
            )
        }
    }
}

/** The banner as Today will show it, links reduced to their words. */
@Composable
private fun BannerPreview(text: String) {
    val shown = text.replace(MarkdownLink, "$1").trim()
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Icon(imageVector = Icons.Outlined.Campaign, contentDescription = null, modifier = Modifier.size(20.dp))
            Column {
                Text(
                    text = stringResource(R.string.native_settings_bannerPreview),
                    style = MaterialTheme.typography.labelSmall,
                )
                Text(
                    text = shown.ifEmpty { stringResource(R.string.native_settings_bannerEmpty) },
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}

@Composable
private fun PinnedCapSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsGroupLabel(stringResource(R.string.settings_menu_pinned_heading))
    HintText(stringResource(R.string.settings_menu_pinned_description))
    SettingsGroup {
        row {
            PinnedCapStepper(
                title = stringResource(R.string.settings_menu_pinned_tasksLabel),
                typed = state.maxPinnedTasks,
                stored = state.user.maxPinnedTasks,
                lessLabel = stringResource(R.string.native_settings_pinnedTasksLess),
                moreLabel = stringResource(R.string.native_settings_pinnedTasksMore),
                onChange = callbacks.onEditMaxPinnedTasks,
            )
        }
        row {
            PinnedCapStepper(
                title = stringResource(R.string.settings_menu_pinned_projectsLabel),
                typed = state.maxPinnedProjects,
                stored = state.user.maxPinnedProjects,
                lessLabel = stringResource(R.string.native_settings_pinnedProjectsLess),
                moreLabel = stringResource(R.string.native_settings_pinnedProjectsMore),
                onChange = callbacks.onEditMaxPinnedProjects,
            )
        }
        row {
            Column(modifier = Modifier.fillMaxWidth()) {
                SettingsNote(
                    stringResource(
                        R.string.settings_menu_pinned_hint,
                        MIN_MAX_PINNED.toString(),
                        MAX_MAX_PINNED.toString(),
                    ),
                )
                // Both caps go to the server in one write, so one button saves both
                // — and only once either of them has moved.
                AnimatedVisibility(
                    visible = state.pinnedCapsEdited,
                    modifier = Modifier.align(Alignment.End).padding(end = 16.dp, bottom = 12.dp),
                ) {
                    Button(onClick = callbacks.onSavePinnedCaps) {
                        Text(stringResource(R.string.settings_menu_pinned_save))
                    }
                }
            }
        }
    }
}

/**
 * One pinning cap, stepped one at a time within the range the server accepts.
 *
 * Steps start from what is being edited, or from what is stored when nothing
 * is, so a cap the server already holds is the starting point rather than zero.
 */
@Composable
private fun PinnedCapStepper(
    title: String,
    typed: String,
    stored: Int,
    lessLabel: String,
    moreLabel: String,
    onChange: (String) -> Unit,
) {
    val value = typed.trim().toIntOrNull() ?: stored
    val step: (Int) -> Unit = { delta -> onChange((value + delta).coerceIn(MIN_MAX_PINNED, MAX_MAX_PINNED).toString()) }
    ListItem(
        headlineContent = { Text(title) },
        trailingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                FilledTonalIconButton(
                    onClick = { step(-1) },
                    enabled = value > MIN_MAX_PINNED,
                    shape = CircleShape,
                ) {
                    Icon(imageVector = Icons.Outlined.Remove, contentDescription = lessLabel)
                }
                Text(
                    text = value.toString(),
                    style = MaterialTheme.typography.titleMedium,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.widthIn(min = 40.dp),
                )
                FilledTonalIconButton(
                    onClick = { step(1) },
                    enabled = value < MAX_MAX_PINNED,
                    shape = CircleShape,
                ) {
                    Icon(imageVector = Icons.Outlined.Add, contentDescription = moreLabel)
                }
            }
        },
        colors = groupRowColors(),
    )
}
