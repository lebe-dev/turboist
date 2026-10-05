package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp

/**
 * The heading that opens a group of settings.
 *
 * Every group carries one, including the ones with a single control, because
 * what a setting *does* is rarely obvious from its switch — and this screen sits
 * over three stores whose differences are exactly the kind of thing a heading
 * has to say out loud.
 */
@Composable
internal fun SettingsSectionHeading(
    titleRes: Int,
    descriptionRes: Int? = null,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 20.dp, bottom = 4.dp)) {
        Text(
            text = stringResource(titleRes),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.primary,
        )
        descriptionRes?.let {
            Text(
                text = stringResource(it),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A setting that is on or off, with the sentence that says what "on" means. */
@Composable
internal fun SettingsSwitchRow(
    titleRes: Int,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    descriptionRes: Int? = null,
) {
    ListItem(
        headlineContent = { Text(stringResource(titleRes)) },
        supportingContent = descriptionRes?.let { { Text(stringResource(it)) } },
        // The whole row is the target and the switch only shows the state, so
        // there is one thing to tap and one thing for a screen reader to
        // announce — the row's own wording — rather than two.
        trailingContent = { Switch(checked = checked, onCheckedChange = null) },
        modifier = Modifier.fillMaxWidth().clickable { onCheckedChange(!checked) },
    )
}
