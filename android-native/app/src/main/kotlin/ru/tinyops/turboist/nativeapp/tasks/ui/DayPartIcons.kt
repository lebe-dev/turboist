package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Bedtime
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.WbSunny
import androidx.compose.material.icons.outlined.WbTwilight
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The phase of the day as a row of four icons, one tap to change it.
 *
 * The answer is one of four and changes often, so a sheet that asks the
 * question again on every change was two taps for a one-tap decision. The icons
 * and their colours are the web client's picker: the chosen one sits on a tinted
 * disc in its own colour, the rest stay muted.
 */
@Composable
internal fun DayPartIconRow(
    dayPart: DayPart,
    onSelect: (DayPart) -> Unit,
    modifier: Modifier = Modifier,
) {
    // A phase this build cannot read sits with "any time", which is where the
    // lists already put it.
    val selected = if (dayPart == DayPart.UNKNOWN) DayPart.NONE else dayPart
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .defaultMinSize(minHeight = ROW_HEIGHT_DP.dp)
                .padding(start = 16.dp, end = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = dayPartIcon(selected),
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = stringResource(R.string.page_task_dayPart),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
        DayPartIconGroup(selected, onSelect)
    }
}

/**
 * The phases as one group of icon buttons, without a label of its own.
 *
 * [compact] is for a line the group shares with other controls — the quick days
 * on the folded "When" card — rather than a row of its own. It draws the icons
 * smaller and leaves "any time" out to fit beside them: tapping the phase that
 * is already chosen takes it back to any time instead.
 */
@Composable
internal fun DayPartIconGroup(
    dayPart: DayPart,
    onSelect: (DayPart) -> Unit,
    modifier: Modifier = Modifier,
    compact: Boolean = false,
) {
    val selected = if (dayPart == DayPart.UNKNOWN) DayPart.NONE else dayPart
    val choices = if (compact) DAY_PART_CHOICES - DayPart.NONE else DAY_PART_CHOICES
    Row(modifier = modifier.selectableGroup()) {
        for (part in choices) {
            val chosen = part == selected
            DayPartIconButton(
                part,
                chosen = chosen,
                compact = compact,
                onSelect = { onSelect(if (compact && chosen) DayPart.NONE else part) },
            )
        }
    }
}

@Composable
private fun DayPartIconButton(
    part: DayPart,
    chosen: Boolean,
    onSelect: () -> Unit,
    compact: Boolean = false,
) {
    val description = stringResource(dayPartLabel(part))
    val tint = if (chosen) dayPartTint(part) else MaterialTheme.colorScheme.onSurfaceVariant
    Box(
        modifier =
            Modifier
                .size(if (compact) 40.dp else 44.dp)
                .clip(CircleShape)
                .selectable(selected = chosen, role = Role.RadioButton, onClick = onSelect)
                .semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        Box(
            modifier =
                Modifier
                    .size(if (compact) 30.dp else 34.dp)
                    .clip(CircleShape)
                    .then(
                        if (chosen) {
                            Modifier.background(MaterialTheme.colorScheme.secondaryContainer)
                        } else {
                            Modifier
                        },
                    ),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = dayPartIcon(part),
                contentDescription = null,
                tint = tint,
                modifier = Modifier.size(20.dp),
            )
        }
    }
}

private fun dayPartIcon(part: DayPart): ImageVector =
    when (part) {
        DayPart.MORNING -> Icons.Outlined.WbTwilight
        DayPart.AFTERNOON -> Icons.Outlined.WbSunny
        DayPart.EVENING -> Icons.Outlined.Bedtime
        DayPart.NONE, DayPart.UNKNOWN -> Icons.Outlined.Schedule
    }

@Composable
private fun dayPartTint(part: DayPart): Color =
    when (part) {
        DayPart.MORNING -> TurboistTheme.accents.morning
        DayPart.AFTERNOON -> TurboistTheme.accents.afternoon
        DayPart.EVENING -> TurboistTheme.accents.evening
        DayPart.NONE, DayPart.UNKNOWN -> MaterialTheme.colorScheme.onSurface
    }
