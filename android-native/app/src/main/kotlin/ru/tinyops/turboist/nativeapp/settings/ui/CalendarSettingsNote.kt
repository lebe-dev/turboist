package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.nativeapp.R

/**
 * What this app can and cannot do with the user's external calendar.
 *
 * A row that says something rather than leading somewhere, because there is
 * nowhere for it to lead. Connecting a calendar is an authorisation handshake
 * that ends by redirecting a browser back to the server, so it is done in the
 * web app; the same is true of choosing which calendars are shown. This client
 * only displays the result — and a settings screen that stayed silent about that
 * would leave the user hunting for a switch that is not here.
 */
@Composable
fun CalendarSettingsNote() {
    ListItem(
        headlineContent = { Text(stringResource(R.string.settings_calendars_heading)) },
        supportingContent = { Text(stringResource(R.string.native_calendar_managed_on_web)) },
        leadingContent = { Icon(imageVector = Icons.Outlined.Event, contentDescription = null) },
    )
}
