package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material3.Badge
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R

/** The rounding of a group's outer corners, and of the seams between its rows. */
private val GroupOuterCorner = 24.dp
private val GroupInnerCorner = 6.dp

/**
 * The name over a group of rows.
 *
 * [trailing] sits on the same line, for the one fact that is true of every row
 * in the group — where they are stored, most often.
 */
@Composable
internal fun SettingsGroupLabel(
    text: String,
    modifier: Modifier = Modifier,
    color: Color = MaterialTheme.colorScheme.primary,
    trailing: @Composable (() -> Unit)? = null,
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(start = 32.dp, end = 32.dp, top = 24.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(text = text, style = MaterialTheme.typography.titleSmall, color = color)
        trailing?.invoke()
    }
}

/** The rows of one group, collected before they are laid out so each knows where it stands. */
internal class SettingsGroupScope {
    internal val rows = mutableListOf<@Composable () -> Unit>()

    fun row(content: @Composable () -> Unit) {
        rows += content
    }
}

/**
 * Rows that belong together, drawn as one rounded block.
 *
 * The rows are separated by a hairline gap rather than a divider, and only the
 * outer corners of the block are fully rounded: the block reads as one thing and
 * each row inside it is still its own target.
 */
@Composable
internal fun SettingsGroup(
    modifier: Modifier = Modifier,
    content: SettingsGroupScope.() -> Unit,
) {
    val rows = SettingsGroupScope().apply(content).rows
    if (rows.isEmpty()) return
    Column(
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        rows.forEachIndexed { index, row ->
            Surface(
                shape = groupRowShape(index, rows.size),
                color = MaterialTheme.colorScheme.surfaceContainer,
                modifier = Modifier.fillMaxWidth(),
            ) { row() }
        }
    }
}

private fun groupRowShape(
    index: Int,
    count: Int,
): Shape {
    val top = if (index == 0) GroupOuterCorner else GroupInnerCorner
    val bottom = if (index == count - 1) GroupOuterCorner else GroupInnerCorner
    return RoundedCornerShape(topStart = top, topEnd = top, bottomStart = bottom, bottomEnd = bottom)
}

/** Lets a row take the colour of the group it sits in rather than painting its own. */
@Composable
internal fun groupRowColors() = ListItemDefaults.colors(containerColor = Color.Transparent)

/** The icon at the start of a row that leads somewhere, on a tinted disc. */
@Composable
internal fun SettingsRowIcon(
    icon: ImageVector,
    tinted: Boolean = true,
) {
    val scheme = MaterialTheme.colorScheme
    Box(
        modifier =
            Modifier
                .size(40.dp)
                .background(if (tinted) scheme.primaryContainer else scheme.surfaceContainerHighest, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = if (tinted) scheme.onPrimaryContainer else scheme.onSurfaceVariant,
            modifier = Modifier.size(22.dp),
        )
    }
}

/**
 * A row that leads to another screen.
 *
 * The chevron says the row goes somewhere rather than toggling something; it
 * carries no description of its own because the row's wording is what a screen
 * reader should announce. [badge] is a count that wants attention.
 */
@Composable
internal fun SettingsNavRow(
    title: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    summary: String? = null,
    icon: ImageVector? = null,
    tintedIcon: Boolean = true,
    badge: Int? = null,
) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = summary?.let { { Text(it) } },
        leadingContent = icon?.let { { SettingsRowIcon(it, tintedIcon) } },
        trailingContent = {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (badge != null && badge > 0) Badge { Text(badge.toString()) }
                Icon(imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null)
            }
        },
        colors = groupRowColors(),
        modifier = modifier.fillMaxWidth().clickable(onClick = onClick),
    )
}

/**
 * A setting that is on or off, with the sentence that says what "on" means.
 *
 * The whole row is the target and the switch only shows the state, so there is
 * one thing to tap and one thing for a screen reader to announce — the row's own
 * wording — rather than two.
 */
@Composable
internal fun SettingsSwitchRow(
    title: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
    description: String? = null,
    icon: ImageVector? = null,
    enabled: Boolean = true,
) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = description?.let { { Text(it) } },
        leadingContent = icon?.let { { Icon(imageVector = it, contentDescription = null) } },
        trailingContent = { Switch(checked = checked, onCheckedChange = null, enabled = enabled) },
        colors = groupRowColors(),
        modifier =
            modifier.fillMaxWidth().toggleable(
                value = checked,
                enabled = enabled,
                role = Role.Switch,
                onValueChange = onCheckedChange,
            ),
    )
}

/** The same switch, named by resources. */
@Composable
internal fun SettingsSwitchRow(
    titleRes: Int,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    descriptionRes: Int? = null,
    enabled: Boolean = true,
) = SettingsSwitchRow(
    title = stringResource(titleRes),
    checked = checked,
    onCheckedChange = onCheckedChange,
    description = descriptionRes?.let { stringResource(it) },
    enabled = enabled,
)

/** A quiet sentence inside a group, for what no control on it can show. */
@Composable
internal fun SettingsNote(
    text: String,
    modifier: Modifier = Modifier,
    icon: ImageVector = Icons.Outlined.Info,
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = text,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * The mark beside a setting kept on this phone rather than in the account.
 *
 * Without it, a switch here and a switch in the next group look the same and
 * mean very different things: one follows the user to every device, the other
 * never leaves this one.
 */
@Composable
internal fun DeviceScopeTag() {
    Surface(shape = CircleShape, color = MaterialTheme.colorScheme.surfaceContainerHighest) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(
                imageVector = Icons.Outlined.PhoneAndroid,
                contentDescription = null,
                modifier = Modifier.size(14.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.native_settings_scopeDevice),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A sentence under a group's name, for the part of the rule its rows cannot show. */
@Composable
internal fun Hint(textRes: Int) = HintText(stringResource(textRes))

/** The same sentence, when the wording takes an argument and is built at the call site. */
@Composable
internal fun HintText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(start = 32.dp, end = 32.dp, bottom = 8.dp),
    )
}

/** Room under the last group, so it never ends flush against the navigation bar. */
@Composable
internal fun SettingsBottomSpace() = Spacer(Modifier.height(32.dp))
