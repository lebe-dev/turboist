package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Label
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.outlined.Flag
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import ru.tinyops.turboist.nativeapp.tasks.ui.DayPartSelector
import ru.tinyops.turboist.nativeapp.tasks.ui.LabelSelector
import ru.tinyops.turboist.nativeapp.tasks.ui.PrioritySelector
import ru.tinyops.turboist.nativeapp.tasks.ui.dayPartLabel
import ru.tinyops.turboist.nativeapp.tasks.ui.priorityLabel
import ru.tinyops.turboist.nativeapp.tasks.ui.priorityTint

/**
 * The editor for whichever value the bar opened, drawn just above the bar.
 *
 * Choosing a priority or a part of the day is one decision, so the panel folds
 * away once it is made; labels are often picked several at a time, so that one
 * stays until its button is tapped again.
 */
@Composable
internal fun AttributePanel(
    panel: QuickAddPanel?,
    state: QuickAddUiState,
    callbacks: QuickAddCallbacks,
    onClose: () -> Unit,
) {
    when (panel) {
        null -> Unit
        QuickAddPanel.PRIORITY ->
            PrioritySelector(
                priority = state.draft.priority,
                onSelect = {
                    callbacks.onPriority(it)
                    onClose()
                },
            )

        QuickAddPanel.DAY_PART ->
            DayPartSelector(
                dayPart = state.draft.dayPart,
                onSelect = {
                    callbacks.onDayPart(it)
                    onClose()
                },
            )

        QuickAddPanel.LABELS ->
            Column(modifier = Modifier.heightIn(max = LABEL_PANEL_MAX_HEIGHT).verticalScroll(rememberScrollState())) {
                LabelSelector(
                    known = state.knownLabels,
                    selected = state.draft.labelNames,
                    onChange = callbacks.onLabels,
                    recentOrder = state.recentLabelOrder,
                )
            }
    }
}

/**
 * The bar pinned to the bottom of the sheet: priority, part of the day and
 * labels, each showing its current value, and the save button.
 *
 * Saving waits while the focus lock would refuse the draft — the card above
 * explains why and offers the way out, and a write that is certain to be
 * refused is not worth queueing.
 */
@Composable
internal fun AttributeBar(
    state: QuickAddUiState,
    open: QuickAddPanel?,
    onToggle: (QuickAddPanel) -> Unit,
    onSubmit: () -> Unit,
    submitDescription: String,
) {
    val draft = state.draft
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 10.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        // The three values share what the save button leaves; the labels, the one
        // with wording of its own, takes whatever room the other two do not need.
        Row(
            modifier = Modifier.weight(1f),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            BarButton(
                icon = Icons.Outlined.Flag,
                iconTint = priorityTint(draft.priority),
                text = priorityLabel(draft.priority),
                description = stringResource(R.string.page_task_priority),
                set = draft.priority != Priority.NONE,
                open = open == QuickAddPanel.PRIORITY,
                onClick = { onToggle(QuickAddPanel.PRIORITY) },
            )
            val anyTime = draft.dayPart == DayPart.NONE || draft.dayPart == DayPart.UNKNOWN
            BarButton(
                icon = Icons.Outlined.Schedule,
                text =
                    if (anyTime) {
                        stringResource(R.string.native_quickAdd_noDayPart)
                    } else {
                        stringResource(dayPartLabel(draft.dayPart))
                    },
                description = stringResource(R.string.page_task_dayPart),
                set = !anyTime,
                open = open == QuickAddPanel.DAY_PART,
                onClick = { onToggle(QuickAddPanel.DAY_PART) },
            )
            if (state.knownLabels.isNotEmpty()) {
                val labels = draft.labelNames
                BarButton(
                    icon = Icons.AutoMirrored.Outlined.Label,
                    text =
                        when (labels.size) {
                            0 -> stringResource(R.string.native_quickAdd_noLabels)
                            1 -> labels.single()
                            else -> labels.first()
                        },
                    // The count of the others is kept apart from the name so that a
                    // long name is what gets shortened, never the count.
                    suffix = if (labels.size > 1) "+${labels.size - 1}" else null,
                    description = stringResource(R.string.page_task_labels),
                    set = labels.isNotEmpty(),
                    open = open == QuickAddPanel.LABELS,
                    onClick = { onToggle(QuickAddPanel.LABELS) },
                    modifier = Modifier.weight(1f, fill = false),
                )
            }
        }
        SubmitButton(
            lines = draft.titleLines.size,
            enabled = draft.canSubmit && !state.submitting && !state.todayLockConflict,
            description = submitDescription,
            onClick = onSubmit,
        )
    }
}

@Composable
private fun BarButton(
    icon: ImageVector,
    text: String,
    description: String,
    set: Boolean,
    open: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    iconTint: Color? = null,
    suffix: String? = null,
) {
    val content = if (set || open) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(12.dp),
        color = if (open) MaterialTheme.colorScheme.surfaceContainerHighest else Color.Transparent,
        contentColor = content,
        border =
            BorderStroke(
                1.dp,
                if (open) MaterialTheme.colorScheme.outline else MaterialTheme.colorScheme.outlineVariant,
            ),
        modifier =
            modifier
                .heightIn(min = 44.dp)
                .semantics {
                    contentDescription = description
                    stateDescription = listOfNotNull(text, suffix).joinToString(" ")
                },
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(icon, contentDescription = null, tint = iconTint ?: content, modifier = Modifier.size(18.dp))
            Text(
                text = text,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false),
            )
            suffix?.let {
                Text(text = it, style = MaterialTheme.typography.labelLarge, maxLines = 1, softWrap = false)
            }
        }
    }
}

/** Saves the draft; when several lines were typed, a badge says how many tasks that makes. */
@Composable
private fun SubmitButton(
    lines: Int,
    enabled: Boolean,
    description: String,
    onClick: () -> Unit,
) {
    BadgedBox(
        badge = {
            if (lines > 1) {
                Badge(
                    containerColor = MaterialTheme.colorScheme.inverseSurface,
                    contentColor = MaterialTheme.colorScheme.inverseOnSurface,
                ) { Text(lines.toString()) }
            }
        },
    ) {
        FilledIconButton(
            onClick = onClick,
            enabled = enabled,
            shape = CircleShape,
            colors =
                IconButtonDefaults.filledIconButtonColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    contentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            modifier = Modifier.size(52.dp).semantics { contentDescription = description },
        ) {
            Icon(Icons.Filled.ArrowUpward, contentDescription = null)
        }
    }
}

/** Labels can be many; past this the panel scrolls rather than pushing the card off screen. */
private val LABEL_PANEL_MAX_HEIGHT = 280.dp
