package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.tasks.ui.ChoiceRow

/** The languages the product is translated into. English is the source and the fallback. */
private val OFFERED_LOCALES = listOf("en" to "English", "ru" to "Русский")

/** The four states of the banner's day-phase gate; the absent one means all day. */
private val BANNER_DAY_PARTS: List<Pair<DayPart?, Int>> =
    listOf(
        null to R.string.settings_banner_dayPart_allDay,
        DayPart.MORNING to R.string.settings_banner_dayPart_morning,
        DayPart.AFTERNOON to R.string.settings_banner_dayPart_afternoon,
        DayPart.EVENING to R.string.settings_banner_dayPart_evening,
    )

/**
 * The preferences that belong to the account.
 *
 * Everything here is one document on the server and follows the user to every
 * device they sign in on — which is why none of it is mixed with the section
 * below it, where the choices stay on this phone.
 *
 * Each control writes the one key it changed, on the spot. There is no "save
 * settings" button for the switches, because there is nothing to batch: a switch
 * that has been flipped has already been decided, and the write goes into the
 * queue whether or not there is a network.
 */
@Composable
internal fun UserPreferencesSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsSectionHeading(R.string.settings_language_heading, R.string.settings_language_description)
    ChoiceRow(label = stringResource(R.string.settings_language_ariaLabel)) {
        for ((code, name) in OFFERED_LOCALES) {
            FilterChip(
                selected = state.user.locale == code,
                onClick = { callbacks.onSetLocale(code) },
                label = { Text(name) },
            )
        }
    }
    Hint(R.string.native_settings_languageDeviceNote)

    SettingsSectionHeading(R.string.settings_privacy_heading, R.string.settings_privacy_description)
    SettingsSwitchRow(
        titleRes = R.string.settings_privacy_toggle,
        checked = state.user.publicView,
        onCheckedChange = callbacks.onSetPublicView,
        descriptionRes = R.string.settings_privacy_hint,
    )

    SettingsSectionHeading(R.string.settings_troiki_heading, R.string.settings_troiki_description)
    SettingsSwitchRow(
        titleRes = R.string.settings_troiki_toggle,
        checked = state.user.troikiEnabled,
        onCheckedChange = callbacks.onSetTroikiEnabled,
    )

    SettingsSectionHeading(R.string.settings_calendars_heading, R.string.settings_calendars_description)
    SettingsSwitchRow(
        titleRes = R.string.settings_calendars_enableLabel,
        checked = state.user.calendarEnabled,
        onCheckedChange = callbacks.onSetCalendarEnabled,
    )
    SettingsSwitchRow(
        titleRes = R.string.settings_calendars_hidePastEvents,
        checked = state.user.calendarHidePastEvents,
        onCheckedChange = callbacks.onSetCalendarHidePastEvents,
        descriptionRes = R.string.settings_calendars_hidePastEventsDescription,
    )
    CalendarSettingsNote()

    BannerSettings(state, callbacks)
    PinnedCapSettings(state, callbacks)
    LabelListSettings(state, callbacks)
}

@Composable
private fun BannerSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsSectionHeading(R.string.settings_banner_heading, R.string.settings_banner_description)
    OutlinedTextField(
        value = state.bannerText,
        onValueChange = callbacks.onEditBannerText,
        label = { Text(stringResource(R.string.settings_banner_textLabel)) },
        placeholder = { Text(stringResource(R.string.settings_banner_textPlaceholder)) },
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    )
    // The text is the one preference here with a save button, because it is
    // typed rather than chosen: writing on every keystroke would queue a request
    // per character and hand the server a half-written sentence.
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
        horizontalArrangement = Arrangement.End,
    ) {
        TextButton(onClick = callbacks.onSaveBannerText, enabled = state.bannerEdited) {
            Text(stringResource(R.string.common_save))
        }
    }
    SettingsSwitchRow(
        titleRes = R.string.settings_banner_publishLabel,
        checked = state.user.bannerPublished,
        onCheckedChange = callbacks.onSetBannerPublished,
    )
    ChoiceRow(label = stringResource(R.string.settings_banner_dayPart_label)) {
        for ((dayPart, wording) in BANNER_DAY_PARTS) {
            FilterChip(
                selected = state.user.bannerDayPart == dayPart,
                onClick = { callbacks.onSetBannerDayPart(dayPart) },
                label = { Text(stringResource(wording)) },
            )
        }
    }
    Hint(R.string.settings_banner_dayPart_hint)
}

@Composable
private fun PinnedCapSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsSectionHeading(R.string.settings_menu_pinned_heading, R.string.settings_menu_pinned_description)
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        OutlinedTextField(
            value = state.maxPinnedTasks,
            onValueChange = callbacks.onEditMaxPinnedTasks,
            label = { Text(stringResource(R.string.settings_menu_pinned_tasksLabel)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        OutlinedTextField(
            value = state.maxPinnedProjects,
            onValueChange = callbacks.onEditMaxPinnedProjects,
            label = { Text(stringResource(R.string.settings_menu_pinned_projectsLabel)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
        horizontalArrangement = Arrangement.End,
    ) {
        TextButton(onClick = callbacks.onSavePinnedCaps, enabled = state.pinnedCapsEdited) {
            Text(stringResource(R.string.settings_menu_pinned_save))
        }
    }
}

/**
 * The two lists of labels the user's preferences name.
 *
 * A label that has never reached the server is not offered: the lists travel
 * back to the server as its own ids, and a device's id means nothing there. The
 * label appears here the moment the queue drains.
 */
@Composable
private fun LabelListSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val labels = state.nameableLabels

    SettingsSectionHeading(R.string.settings_weekly_heading, R.string.settings_weekly_description)
    LabelChoices(
        labels = labels,
        chosenServerIds = state.user.weeklyUnplannedExcludedLabelIds,
        emptyRes = R.string.settings_weekly_empty,
        onToggle = callbacks.onToggleWeeklyExcludedLabel,
    )

    SettingsSectionHeading(R.string.settings_project_bugLabelsHeading, R.string.settings_project_bugLabelsDescription)
    LabelChoices(
        labels = labels,
        chosenServerIds = state.user.bugLabelIds,
        emptyRes = R.string.settings_project_bugLabelsEmpty,
        onToggle = callbacks.onToggleBugLabel,
    )
}

@Composable
private fun LabelChoices(
    labels: List<Label>,
    chosenServerIds: List<Long>,
    emptyRes: Int,
    onToggle: (Long) -> Unit,
) {
    if (labels.isEmpty()) {
        Hint(emptyRes)
        return
    }
    ChoiceRow(label = stringResource(R.string.common_labels)) {
        for (label in labels) {
            val serverId = label.serverId ?: continue
            FilterChip(
                selected = serverId in chosenServerIds,
                onClick = { onToggle(serverId) },
                label = { Text(label.name) },
            )
        }
    }
}

/** A sentence under a control, for the part of the rule the control cannot show. */
@Composable
internal fun Hint(textRes: Int) = HintText(stringResource(textRes))

/** The same sentence, when the wording takes an argument and is built at the call site. */
@Composable
internal fun HintText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    )
}
