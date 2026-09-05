package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState

/**
 * What this build is, and the documents that govern using it.
 *
 * The version is read from the installed package, so it is the artifact's own
 * answer rather than a constant that can drift from it; on a build made by a
 * machine that supplied one, it carries the commit as well, which is what turns
 * a bug report into something anyone can reproduce.
 *
 * The two documents are opened on the server the user connected to rather than
 * bundled. The product is self-hosted, so the terms that apply are the ones that
 * installation publishes — and a copy inside the app would show text nobody
 * agreed to and go stale the moment the server's is edited. They are not offered
 * at all until there is a server to open them on.
 */
@Composable
internal fun AboutSection(state: SettingsUiState) {
    val links = LocalUriHandler.current

    SettingsSectionHeading(R.string.native_settings_aboutHeading)
    ListItem(
        headlineContent = { Text(stringResource(R.string.settings_version_heading)) },
        supportingContent = { Text(stringResource(R.string.settings_version_description)) },
        leadingContent = { Icon(imageVector = Icons.Outlined.Info, contentDescription = null) },
        trailingContent = {
            Text(state.version.ifEmpty { stringResource(R.string.native_settings_versionUnknown) })
        },
    )
    if (state.termsUrl.isNotEmpty()) {
        LegalRow(R.string.legal_tos_title) { links.openUri(state.termsUrl) }
    }
    if (state.privacyUrl.isNotEmpty()) {
        LegalRow(R.string.legal_privacy_title) { links.openUri(state.privacyUrl) }
    }
}

@Composable
private fun LegalRow(
    titleRes: Int,
    onOpen: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(stringResource(titleRes)) },
        // The icon says the row leaves the app for a browser; it carries no
        // description of its own because the row's wording is what a screen
        // reader should announce.
        trailingContent = {
            Icon(imageVector = Icons.AutoMirrored.Outlined.OpenInNew, contentDescription = null)
        },
        modifier = Modifier.clickable(onClick = onOpen),
    )
}
