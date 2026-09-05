package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * A group of related fields, drawn as one surface.
 *
 * The detail screen shows a task from half a dozen angles at once — when it is
 * due, how it is planned, what it is tagged with, what it has been through — and
 * a flat run of rows with dividers between them says nothing about which of
 * those a row belongs to. The card says it: things inside one are about the same
 * question, and the gap between two cards is where one question ends.
 */
@Composable
fun DetailCard(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        shape = RoundedCornerShape(CARD_CORNER_DP.dp),
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp),
    ) {
        Column(modifier = Modifier.padding(vertical = 4.dp), content = content)
    }
}

/**
 * What the card under it is about, with the control that changes the whole group
 * on the right.
 *
 * It sits outside the card rather than as its first row: a heading inside a
 * surface reads as another field of it, and the point of the heading is to say
 * where one group of fields stops.
 */
@Composable
fun SectionHeading(
    text: String,
    modifier: Modifier = Modifier,
    action: @Composable RowScope.() -> Unit = {},
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(start = 20.dp, end = 20.dp, top = 20.dp, bottom = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        action()
    }
}

/**
 * One fact about a task: what it is on the left, what it says on the right.
 *
 * The whole row is the control where there is something to open, because a value
 * that can be changed should be reachable by aiming at the value itself rather
 * than at an icon next to it. The chevron appears for exactly that reason — it
 * is the row's promise that tapping it leads somewhere, and a row with nothing
 * behind it does not make the promise.
 *
 * [supporting] is a second line under the label, for the one thing a value
 * cannot say about itself: when the next repeat falls, say. [trailing] is what
 * that promise looks like — a chevron to the side for a row that opens a sheet,
 * and one pointing down for a row that unfolds in place.
 */
@Composable
fun DetailRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    leading: ImageVector? = null,
    leadingTint: Color? = null,
    supporting: String? = null,
    valueTint: Color? = null,
    trailing: ImageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
    onClick: (() -> Unit)? = null,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
                .defaultMinSize(minHeight = ROW_HEIGHT_DP.dp)
                .padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            Icon(
                imageVector = leading,
                contentDescription = null,
                tint = leadingTint ?: MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(text = label, style = MaterialTheme.typography.bodyMedium)
            if (supporting != null) {
                Text(
                    text = supporting,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Text(
            text = value,
            style = MaterialTheme.typography.bodyMedium,
            color = valueTint ?: MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false),
        )
        if (onClick != null) {
            Icon(
                imageVector = trailing,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(20.dp),
            )
        }
    }
}

/** A yes-or-no fact about a task, changed in place. */
@Composable
fun DetailSwitchRow(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
    leading: ImageVector? = null,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .defaultMinSize(minHeight = ROW_HEIGHT_DP.dp)
                .padding(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            Icon(
                imageVector = leading,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
        Switch(
            checked = checked,
            onCheckedChange = onCheckedChange,
            modifier = Modifier.semantics { contentDescription = label },
        )
    }
}

/**
 * The four priority levels, as one connected group.
 *
 * Shown open rather than behind a sheet: a priority is the field most often
 * changed on this screen, and the current level reads at a glance only when the
 * others are beside it. The group is one control rather than four chips because
 * the levels are a scale — exactly one of them is true at a time — and a row of
 * chips says the opposite.
 *
 * The chosen level is filled with its own signalling colour rather than with the
 * scheme's accent. The colour is what a user reads first on every list in both
 * clients, and a picker that drops it would be the one place in the product
 * where P1 is not red.
 *
 * [locked] is set when the task's project stands in the daily plan and decides
 * the priority itself. The levels stay visible so the current one still reads,
 * but none of them can be picked and the reason is spelled out underneath — an
 * unexplained dead control would look like a broken screen.
 */
@Composable
fun PrioritySelector(
    priority: Priority,
    onSelect: (Priority) -> Unit,
    modifier: Modifier = Modifier,
    locked: Boolean = false,
) {
    Column(modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.padding(bottom = 8.dp),
        ) {
            Text(
                text = stringResource(R.string.page_task_priority),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (locked) {
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(14.dp),
                )
            }
        }
        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
            PRIORITY_CHOICES.forEachIndexed { index, level ->
                val text = priorityLabel(level)
                val tint = priorityTint(level)
                SegmentedButton(
                    selected = level == priority,
                    onClick = { onSelect(level) },
                    enabled = !locked,
                    shape = SegmentedButtonDefaults.itemShape(index = index, count = PRIORITY_CHOICES.size),
                    colors =
                        SegmentedButtonDefaults.colors(
                            activeContainerColor = tint.copy(alpha = SELECTED_LEVEL_ALPHA),
                            activeContentColor = tint,
                            activeBorderColor = Color.Transparent,
                            inactiveContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                            inactiveContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                            inactiveBorderColor = Color.Transparent,
                            disabledActiveContainerColor = tint.copy(alpha = SELECTED_LEVEL_ALPHA),
                            disabledActiveContentColor = tint,
                            disabledActiveBorderColor = Color.Transparent,
                            disabledInactiveContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                            disabledInactiveContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                            disabledInactiveBorderColor = Color.Transparent,
                        ),
                    icon = { LevelDot(tint) },
                    label = { Text(text) },
                    modifier = Modifier.semantics { contentDescription = text },
                )
            }
        }
        if (locked) {
            Text(
                text = stringResource(R.string.page_task_priorityLockedByTroiki),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
    }
}

/** The mark a level carries in every list, repeated on the control that sets it. */
@Composable
private fun LevelDot(tint: Color) {
    Surface(
        color = tint,
        shape = CircleShape,
        modifier = Modifier.size(8.dp),
        content = {},
    )
}

/** The phase of the day a task is meant for. */
@Composable
fun DayPartSelector(
    dayPart: DayPart,
    onSelect: (DayPart) -> Unit,
    modifier: Modifier = Modifier,
) {
    ChoiceRow(
        label = stringResource(R.string.page_task_dayPart),
        modifier = modifier,
    ) {
        for (part in DAY_PART_CHOICES) {
            val text = stringResource(dayPartLabel(part))
            FilterChip(
                selected = part == dayPart,
                onClick = { onSelect(part) },
                label = { Text(text) },
                modifier = Modifier.semantics { contentDescription = text },
            )
        }
    }
}

/**
 * Every label in the workspace, with the ones on this task marked.
 *
 * The whole set is offered rather than a search box: labels are few and
 * deliberately so, and a set the user can see is a set they can keep tidy.
 */
@Composable
fun LabelSelector(
    known: List<Label>,
    selected: List<String>,
    onChange: (List<String>) -> Unit,
    modifier: Modifier = Modifier,
) {
    if (known.isEmpty()) return
    ChoiceRow(
        label = stringResource(R.string.page_task_labels),
        modifier = modifier,
    ) {
        for (label in known) {
            val on = label.name in selected
            FilterChip(
                selected = on,
                onClick = { onChange(if (on) selected - label.name else selected + label.name) },
                label = { Text(label.name) },
                modifier = Modifier.semantics { contentDescription = label.name },
            )
        }
    }
}

/**
 * A named row of choices, wrapping onto as many lines as it needs.
 *
 * [note] is a line under the choices, for when the row cannot be used and the
 * user is owed the reason.
 */
@Composable
internal fun ChoiceRow(
    label: String,
    modifier: Modifier = Modifier,
    note: String? = null,
    content: @Composable FlowRowScope.() -> Unit,
) {
    Column(modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp),
            content = content,
        )
        if (note != null) {
            Text(
                text = note,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A moment, written the way the device writes dates and times. */
@Composable
fun momentLabel(
    at: Long?,
    zone: ZoneId,
): String {
    at ?: return stringResource(R.string.native_task_unset)
    return Instant
        .ofEpochMilli(at)
        .atZone(zone)
        .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT))
}

/** The short code a priority is shown as, the same scale the web client uses. */
@Composable
fun priorityLabel(priority: Priority): String =
    when (priority) {
        Priority.HIGH -> stringResource(R.string.native_priority_p1)
        Priority.MEDIUM -> stringResource(R.string.native_priority_p2)
        Priority.LOW -> stringResource(R.string.native_priority_p3)
        Priority.NONE -> stringResource(R.string.native_priority_p4)
        // A level this build does not recognise is shown as unset rather than
        // guessed at: the task must still render, and naming it wrongly is worse
        // than admitting the app cannot read it.
        Priority.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

@Composable
fun planLabel(planState: PlanState): String =
    when (planState) {
        PlanState.WEEK -> stringResource(R.string.task_weekPlannedLabel)
        PlanState.BACKLOG -> stringResource(R.string.task_backlogLabel)
        PlanState.NONE -> stringResource(R.string.native_task_planNone)
        PlanState.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

/** Where the task stands: still open, finished, or closed without being done. */
@Composable
fun statusLabel(status: TaskStatus): String =
    when (status) {
        TaskStatus.OPEN -> stringResource(R.string.native_task_statusOpen)
        TaskStatus.COMPLETED -> stringResource(R.string.native_task_statusCompleted)
        TaskStatus.CANCELLED -> stringResource(R.string.native_task_statusCancelled)
        TaskStatus.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

/** How much of the chosen level's own colour its segment is filled with. */
private const val SELECTED_LEVEL_ALPHA = 0.20f

/** The corner every card on the screen is cut with. */
internal const val CARD_CORNER_DP = 24

/** The height every row inside a card is at least, which is also its hit target. */
internal const val ROW_HEIGHT_DP = 56

/** The levels a user can choose, most urgent first. The unreadable one is not offered. */
internal val PRIORITY_CHOICES = listOf(Priority.HIGH, Priority.MEDIUM, Priority.LOW, Priority.NONE)

internal val DAY_PART_CHOICES = listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE)

internal val PLAN_CHOICES = listOf(PlanState.WEEK, PlanState.BACKLOG, PlanState.NONE)
