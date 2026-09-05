package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.outlined.CheckBox
import androidx.compose.material.icons.outlined.CheckBoxOutlineBlank
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.material.icons.outlined.EventAvailable
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.HourglassEmpty
import androidx.compose.material.icons.outlined.Inventory2
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Psychology
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material.icons.outlined.Repeat
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId

/**
 * One task, as a list draws it.
 *
 * The control on the left is the whole story of what can be done with the row.
 * A task something still holds up shows a padlock instead of an empty box and
 * cannot be ticked: the rule is the server's, the device repeats it, and saying
 * so before the tap is the only version of it the user is not misled by. A task
 * created while offline shows that it is still waiting to be sent — it is real
 * work and behaves like any other row, but it has no address on the server yet,
 * and hiding that would make its absence elsewhere look like a bug.
 *
 * Everything else on the row is a fact about the task worth seeing without
 * opening it: how urgent it is, when it is due, what it is tagged with, where it
 * lives, whether it repeats, whether it has been put off, whether it is linked
 * to other work, and whether it has been planned for the week or parked.
 */
@Composable
fun TaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    selectionMode: Boolean,
    selected: Boolean,
    onToggleComplete: () -> Unit,
    onOpen: () -> Unit,
    onSelectToggle: () -> Unit,
    onStartSelection: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val task = row.task
    val completed = task.status == TaskStatus.COMPLETED
    val blocked = task.isBlocked && !completed
    val tint = priorityTint(task.priority)

    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .then(
                    if (selected) {
                        Modifier.background(MaterialTheme.colorScheme.secondaryContainer)
                    } else {
                        Modifier
                    },
                )
                .combinedClickable(
                    onClick = { if (selectionMode) onSelectToggle() else onOpen() },
                    onLongClick = onStartSelection,
                )
                .padding(start = (row.depth * INDENT_STEP_DP).dp + 8.dp, end = 8.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.Top,
    ) {
        if (selectionMode) {
            IconButton(onClick = onSelectToggle) {
                Icon(
                    imageVector = if (selected) Icons.Outlined.CheckBox else Icons.Outlined.CheckBoxOutlineBlank,
                    contentDescription =
                        stringResource(
                            if (selected) R.string.selection_unselectTask else R.string.selection_selectTask,
                        ),
                )
            }
        }

        CompletionControl(
            completed = completed,
            blocked = blocked,
            tint = tint,
            onToggleComplete = onToggleComplete,
        )

        Column(modifier = Modifier.padding(start = 4.dp, top = 8.dp)) {
            TitleLine(task = task, completed = completed)
            MetaLine(
                task = task,
                projectTitle = row.projectTitle,
                zone = zone,
                today = today,
                completed = completed,
            )
        }
    }
}

/**
 * The tick box, and the two things it can be instead of one.
 *
 * A blocked task gets a padlock rather than a greyed-out box, because a greyed
 * box still reads as "a box you may tick later"; the padlock says what is
 * actually true. Its colour still carries the priority, so nothing is lost by
 * the swap.
 */
@Composable
private fun CompletionControl(
    completed: Boolean,
    blocked: Boolean,
    tint: Color,
    onToggleComplete: () -> Unit,
) {
    IconButton(onClick = onToggleComplete, enabled = !blocked) {
        when {
            blocked ->
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = stringResource(R.string.task_blockedTooltip),
                    tint = tint,
                )

            completed ->
                Icon(
                    imageVector = Icons.Filled.CheckCircle,
                    contentDescription = stringResource(R.string.task_markIncomplete),
                    tint = MaterialTheme.colorScheme.outline,
                )

            else ->
                Icon(
                    imageVector = Icons.Outlined.Circle,
                    contentDescription = stringResource(R.string.task_markComplete),
                    tint = tint,
                )
        }
    }
}

/** The title, plus the markers that belong to the task as a whole. */
@Composable
private fun TitleLine(
    task: Task,
    completed: Boolean,
) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            text = task.title,
            style = MaterialTheme.typography.bodyMedium,
            color =
                if (completed) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
            textDecoration = if (completed) TextDecoration.LineThrough else null,
            modifier = Modifier.weight(1f, fill = false),
        )
        if (task.isComplex) {
            MarkerIcon(Icons.Outlined.Psychology, R.string.task_complexMarker, TurboistTheme.accents.demanding)
        }
        // A pinned task is one the user asked to keep in front of them, and it
        // sorts to the top of every list it is in. The marker says why it is
        // there, so the ordering does not read as arbitrary.
        if (task.isPinned) {
            MarkerIcon(Icons.Outlined.PushPin, R.string.nav_pinned, MaterialTheme.colorScheme.primary)
        }
        if (task.isPrivate) {
            MarkerIcon(Icons.Outlined.Lock, R.string.common_privateMarker, MaterialTheme.colorScheme.outline)
        }
        if (task.serverId == null) AwaitingSendBadge()
    }
}

/**
 * Says a task has not reached the server yet.
 *
 * Shown for anything the replica holds without a server id, which is exactly the
 * set of rows created on this device and not yet accepted. It disappears by
 * itself when the queue drains and the row comes back with an id, with nothing
 * to clear and no state of its own to go stale.
 */
@Composable
private fun AwaitingSendBadge() {
    Row(
        modifier =
            Modifier
                .background(TurboistTheme.accents.pendingContainer, RoundedCornerShape(8.dp))
                .padding(horizontal = 6.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.HourglassEmpty,
            contentDescription = null,
            tint = TurboistTheme.accents.pending,
            modifier = Modifier.size(12.dp),
        )
        Text(
            text = stringResource(R.string.offline_awaitingSend),
            style = MaterialTheme.typography.labelSmall,
            color = TurboistTheme.accents.pending,
        )
    }
}

/** The facts under the title: when, where, what it is tagged with, what it is entangled with. */
@Composable
private fun MetaLine(
    task: Task,
    projectTitle: String?,
    zone: ZoneId,
    today: LocalDate,
    completed: Boolean,
) {
    val due = dueLabel(task, zone, today)
    val recurring = task.recurrenceRule != null || task.sourceTaskLocalId != null
    val relations = task.relationSummary.total
    val postponed = task.postponeCount >= POSTPONE_WORTH_SHOWING
    val plan = task.planState
    val nothingToShow =
        due == null && !recurring && relations == 0 && !postponed && projectTitle == null &&
            task.labels.isEmpty() && plan != PlanState.WEEK && plan != PlanState.BACKLOG
    if (nothingToShow) return

    FlowRow(
        modifier = Modifier.padding(top = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        if (recurring) {
            MarkerIcon(
                Icons.Outlined.Repeat,
                R.string.task_recurringLabel,
                // A finished task's marks stop competing with the open work
                // around them; the fact that it repeats is history by then.
                if (completed) MaterialTheme.colorScheme.onSurfaceVariant else TurboistTheme.accents.recurring,
            )
        }
        if (due != null) DueText(due, if (completed) DueUrgency.ORDINARY else dueUrgency(task.dueAt, zone, today))
        if (postponed) MetaText(stringResource(R.string.task_postponedTimes, task.postponeCount))
        if (relations > 0) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(2.dp)) {
                MarkerIcon(Icons.Outlined.Link, R.string.task_relationCountLabel, argument = relations)
                MetaText(relations.toString())
            }
        }
        if (projectTitle != null) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(2.dp)) {
                MarkerIcon(Icons.Outlined.Folder, R.string.nav_projects)
                MetaText(projectTitle)
            }
        }
        for (label in task.labels) LabelChip(label.name)
        if (plan == PlanState.WEEK) MarkerIcon(Icons.Outlined.EventAvailable, R.string.task_weekPlannedLabel)
        if (plan == PlanState.BACKLOG) MarkerIcon(Icons.Outlined.Inventory2, R.string.task_backlogLabel)
    }
}

@Composable
private fun MarkerIcon(
    icon: ImageVector,
    descriptionRes: Int,
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    argument: Int? = null,
) {
    Icon(
        imageVector = icon,
        contentDescription =
            if (argument == null) {
                stringResource(descriptionRes)
            } else {
                stringResource(descriptionRes, argument)
            },
        tint = tint,
        modifier = Modifier.size(14.dp),
    )
}

/**
 * The due date, emphasised by how close it is.
 *
 * Late work is called out in the scheme's alarm colour and today's is given the
 * page's own text colour, which on a line of otherwise muted facts is what makes
 * it the one the eye lands on. Everything further out stays muted: a date three
 * weeks away is context, not a demand.
 */
@Composable
private fun DueText(
    text: String,
    urgency: DueUrgency,
) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color =
            when (urgency) {
                DueUrgency.OVERDUE -> MaterialTheme.colorScheme.error
                DueUrgency.TODAY -> MaterialTheme.colorScheme.onSurface
                DueUrgency.ORDINARY -> MaterialTheme.colorScheme.onSurfaceVariant
            },
    )
}

@Composable
private fun MetaText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/**
 * One tag, drawn as the web client draws it: a filled pill rather than an
 * outlined one. A row can carry several, and several outlines in a line read as
 * a row of empty boxes.
 */
@Composable
private fun LabelChip(name: String) {
    Text(
        text = name,
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier =
            Modifier
                .background(MaterialTheme.colorScheme.surfaceContainerHigh, CircleShape)
                .padding(horizontal = 8.dp, vertical = 1.dp),
    )
}

/** How far one level of nesting shifts a row, in density-independent pixels. */
private const val INDENT_STEP_DP = 16

/**
 * How many postponements are worth showing. One is a normal day; a habit of
 * putting the same task off is the thing worth noticing.
 */
private const val POSTPONE_WORTH_SHOWING = 2
