package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice

/** The languages the product is translated into. English is the source and the fallback. */
internal val OFFERED_LOCALES = listOf("en" to "English", "ru" to "Русский")

/** The three answers to "which colours", in the order they read best. */
internal val THEME_CHOICES: List<Pair<ThemeChoice, Int>> =
    listOf(
        ThemeChoice.SYSTEM to R.string.settings_theme_system,
        ThemeChoice.LIGHT to R.string.settings_theme_light,
        ThemeChoice.DARK to R.string.settings_theme_dark,
    )

/**
 * How the app speaks and looks, and what it hides from a screenshot.
 *
 * The language and Public view belong to the account and follow the user to
 * every device; the theme stays on this phone. They sit together because they
 * answer the same question — how the app appears — and the theme's group says
 * out loud that it is the exception.
 *
 * Each control writes the one key it changed, on the spot. There is nothing to
 * batch: a choice that has been made has already been decided, and the write goes
 * into the queue whether or not there is a network.
 */
@Composable
internal fun GeneralSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsGroupLabel(stringResource(R.string.settings_language_heading))
    SettingsGroup {
        row {
            Column(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    OFFERED_LOCALES.forEachIndexed { index, (code, name) ->
                        SegmentedButton(
                            selected = state.user.locale == code,
                            onClick = { callbacks.onSetLocale(code) },
                            shape = SegmentedButtonDefaults.itemShape(index, OFFERED_LOCALES.size),
                            label = { Text(name) },
                        )
                    }
                }
            }
        }
        row { SettingsNote(stringResource(R.string.native_settings_languageDeviceNote)) }
    }

    SettingsGroupLabel(
        text = stringResource(R.string.settings_theme_heading),
        trailing = { DeviceScopeTag() },
    )
    SettingsGroup {
        row {
            Column(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    THEME_CHOICES.forEachIndexed { index, (choice, wording) ->
                        SegmentedButton(
                            selected = state.device.theme == choice,
                            onClick = { callbacks.onSetTheme(choice) },
                            shape = SegmentedButtonDefaults.itemShape(index, THEME_CHOICES.size),
                            label = { Text(stringResource(wording)) },
                        )
                    }
                }
            }
        }
    }

    SettingsGroupLabel(stringResource(R.string.native_settings_privacyGroup))
    SettingsGroup {
        row {
            SettingsSwitchRow(
                title = stringResource(R.string.settings_privacy_toggle),
                checked = state.user.publicView,
                onCheckedChange = callbacks.onSetPublicView,
                description = stringResource(R.string.settings_privacy_hint),
                icon = Icons.Outlined.Visibility,
            )
        }
    }
}
