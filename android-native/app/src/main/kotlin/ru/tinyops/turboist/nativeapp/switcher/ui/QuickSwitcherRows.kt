package ru.tinyops.turboist.nativeapp.switcher.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.History
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.switcher.SwitcherRow

// The rows of the quick switcher's list: a found task or project, and an "add" row.

private fun SwitcherRow.isFinished(): Boolean = this is SwitcherRow.TaskRow && task.status != TaskStatus.OPEN

private fun SwitcherRow.isStruck(): Boolean = this is SwitcherRow.TaskRow && task.status == TaskStatus.COMPLETED

private fun SwitcherRow.icon(): ImageVector =
    when {
        this is SwitcherRow.ProjectRow -> Icons.Outlined.Folder
        isFinished() -> Icons.Outlined.CheckCircle
        else -> Icons.Outlined.Circle
    }

private fun SwitcherRow.title(): String =
    when (this) {
        is SwitcherRow.TaskRow -> task.title
        is SwitcherRow.ProjectRow -> project.title
    }

/** The second line: "Project" for a project, where a task lives for a task. */
@Composable
private fun SwitcherRow.subtitle(): String? =
    when (this) {
        is SwitcherRow.ProjectRow -> stringResource(R.string.quickSwitcher_kind_project)
        is SwitcherRow.TaskRow -> if (task.inboxId != null) stringResource(R.string.nav_inbox) else projectTitle
    }

@Composable
internal fun ResultRow(
    row: SwitcherRow,
    onClick: () -> Unit,
) {
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Icon(row.icon(), contentDescription = null, tint = muted, modifier = Modifier.size(20.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = row.title(),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                textDecoration = if (row.isStruck()) TextDecoration.LineThrough else null,
                color = if (row.isFinished()) muted else MaterialTheme.colorScheme.onSurface,
            )
            row.subtitle()?.let { subtitle ->
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = muted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        if (row.recent) {
            Icon(
                Icons.Outlined.History,
                contentDescription = stringResource(R.string.quickSwitcher_recentBadge),
                tint = muted,
                modifier = Modifier.size(16.dp),
            )
        }
    }
}

@Composable
internal fun CreateRow(
    icon: ImageVector,
    text: String,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    trailing: String? = null,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        modifier =
            modifier
                .fillMaxWidth()
                .clickable(enabled = enabled, onClick = onClick)
                .padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(20.dp))
        Text(
            text = text,
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (trailing != null) {
            Text(
                text = trailing,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** The label of a part of the day, as the task screens name it. */
internal fun dayPartLabel(part: DayPart): Int =
    when (part) {
        DayPart.MORNING -> R.string.task_dayPart_morning
        DayPart.AFTERNOON -> R.string.task_dayPart_afternoon
        DayPart.EVENING -> R.string.task_dayPart_evening
        else -> R.string.task_dayPart_anytime
    }
