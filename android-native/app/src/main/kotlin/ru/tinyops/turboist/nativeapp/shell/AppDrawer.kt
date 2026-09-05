package ru.tinyops.turboist.nativeapp.shell

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.NavigationDrawerItemDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.navigation.DrawerDestination
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The drawer body: the product's destination list, grouped, with the counters
 * that matter next to the entries they belong to.
 *
 * It renders straight off [DrawerDestination.grouped], so a destination added
 * to the catalog appears here without this file changing.
 *
 * [dailyPlanEnabled] is the user's preference for the daily plan, which the
 * catalog uses to decide whether the plan is one of this installation's places
 * at all.
 */
@Composable
fun AppDrawerContent(
    counts: DrawerCounts,
    selected: (DrawerDestination) -> Boolean,
    onSelect: (DrawerDestination) -> Unit,
    onSignOut: () -> Unit,
    modifier: Modifier = Modifier,
    dailyPlanEnabled: Boolean = false,
) {
    Column(
        modifier = modifier.verticalScroll(rememberScrollState()).padding(vertical = 12.dp),
    ) {
        Text(
            text = stringResource(R.string.native_app_name),
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.padding(horizontal = 28.dp, vertical = 12.dp),
        )

        DrawerDestination.grouped(dailyPlanEnabled).forEach { (group, destinations) ->
            Text(
                text = stringResource(group.titleRes),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 28.dp, top = 16.dp, bottom = 4.dp),
            )
            destinations.forEach { destination ->
                NavigationDrawerItem(
                    selected = selected(destination),
                    onClick = { onSelect(destination) },
                    icon = {
                        Icon(
                            imageVector = destination.icon,
                            contentDescription = null,
                        )
                    },
                    label = { Text(stringResource(destination.labelRes)) },
                    badge = {
                        BadgeLabel(counts.badgeFor(destination))
                    },
                    modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
                )
            }
        }

        // Signing out is the one drawer entry that is not a place to go, so it is
        // separated from the destination list rather than filed under a group.
        HorizontalDivider(modifier = Modifier.padding(horizontal = 28.dp, vertical = 12.dp))
        NavigationDrawerItem(
            selected = false,
            onClick = onSignOut,
            icon = { Icon(imageVector = Icons.AutoMirrored.Filled.Logout, contentDescription = null) },
            label = { Text(stringResource(R.string.sidebar_logOut)) },
            modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
        )
    }
}

/** Renders a drawer badge; a destination with no counter draws nothing. */
@Composable
private fun BadgeLabel(badge: DrawerBadge?) {
    when (badge) {
        null -> Unit
        is DrawerBadge.Count -> BadgeText(badge.value.toString())
        is DrawerBadge.Ratio -> BadgeText(stringResource(R.string.native_count_ratio, badge.current, badge.limit))
    }
}

@Composable
private fun BadgeText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Preview(showBackground = true)
@Composable
private fun AppDrawerContentPreview() {
    TurboistTheme {
        AppDrawerContent(
            counts = DrawerCounts(inbox = 3, weekPlanned = 5, weekLimit = 12),
            selected = { it == DrawerDestination.Today },
            onSelect = {},
            onSignOut = {},
            dailyPlanEnabled = true,
        )
    }
}
