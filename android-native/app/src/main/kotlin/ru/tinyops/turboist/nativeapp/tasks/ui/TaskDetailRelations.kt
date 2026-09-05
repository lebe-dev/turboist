package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskRelationCandidate
import ru.tinyops.turboist.nativeapp.tasks.TaskRelationRef

/**
 * What this task waits for, what waits for it, and what is merely worth reading
 * beside it.
 *
 * The three headings are the three things a person says about two pieces of
 * work; the store keeps two kinds of edge and the direction tells the first two
 * apart. Each link names the task at the other end, because a person recognises
 * a title and not an id, and tapping it goes there — a list of blockers you
 * cannot reach is a list of complaints.
 *
 * The whole section is drawn from the replica, so it is right with no
 * connection: a blocker finished on a plane clears its padlock here immediately,
 * and the link the user adds is on screen before the request for it has been
 * sent.
 */
@Composable
fun TaskDetailRelations(
    relations: List<TaskRelationRef>,
    candidates: List<TaskRelationCandidate>,
    onSearch: (String) -> Unit,
    onAdd: (Long, TaskRelationGroup) -> Unit,
    onRemove: (Long) -> Unit,
    onOpen: (Long) -> Unit,
    modifier: Modifier = Modifier,
) {
    var picking by remember { mutableStateOf(false) }

    SectionHeading(text = stringResource(R.string.page_task_relations), modifier = modifier) {
        TextButton(onClick = { picking = true }) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = null,
                modifier = Modifier.size(18.dp),
            )
            Text(
                text = stringResource(R.string.page_task_addRelation),
                modifier = Modifier.padding(start = 4.dp),
            )
        }
    }
    DetailCard {
        if (relations.isEmpty()) {
            Text(
                text = stringResource(R.string.page_task_relationEmpty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            )
        }
        for (group in TaskRelationGroup.entries) {
            val links = relations.filter { it.group == group }
            if (links.isEmpty()) continue
            Text(
                text = stringResource(groupLabel(group)),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 2.dp),
            )
            for (link in links) RelationRow(link, onRemove, onOpen)
        }
    }

    if (!picking) return
    AddTaskRelationSheet(
        candidates = candidates,
        onSearch = onSearch,
        onPick = { peerLocalId, group ->
            picking = false
            onAdd(peerLocalId, group)
        },
        onDismiss = {
            picking = false
            onSearch("")
        },
    )
}

/**
 * One link: what it says, the task at the other end, and the way to undo it.
 *
 * A peer that is finished or abandoned is struck through rather than dropped.
 * The link is still a fact about this task — it is why the work was waiting —
 * and removing it from view would leave the user wondering what happened to it.
 */
@Composable
private fun RelationRow(
    link: TaskRelationRef,
    onRemove: (Long) -> Unit,
    onOpen: (Long) -> Unit,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable { onOpen(link.peerLocalId) }
                .padding(start = 16.dp, end = 4.dp, top = 2.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = groupIcon(link.group),
            contentDescription = null,
            tint = groupTint(link),
            modifier = Modifier.size(16.dp),
        )
        Text(
            text = link.peerTitle,
            style = MaterialTheme.typography.bodyMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textDecoration = if (link.peerSettled) TextDecoration.LineThrough else null,
            color =
                if (link.peerSettled) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    Color.Unspecified
                },
            modifier = Modifier.weight(1f).padding(start = 8.dp, top = 8.dp, bottom = 8.dp),
        )
        IconButton(onClick = { onRemove(link.relationLocalId) }) {
            Icon(
                imageVector = Icons.Filled.Close,
                contentDescription = stringResource(R.string.page_task_relationRemove),
                modifier = Modifier.size(16.dp),
            )
        }
    }
}

/**
 * Choosing the other end of a link, and what the link says.
 *
 * The kind is chosen first and stays chosen while the user searches, because it
 * is the question they already know the answer to — they came here to say "this
 * is waiting for something", and the search is only how they name it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AddTaskRelationSheet(
    candidates: List<TaskRelationCandidate>,
    onSearch: (String) -> Unit,
    onPick: (Long, TaskRelationGroup) -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var group by remember { mutableStateOf(TaskRelationGroup.BLOCKED_BY) }
    var typed by remember { mutableStateOf("") }

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(modifier = Modifier.fillMaxWidth().padding(bottom = 24.dp)) {
            Text(
                text = stringResource(R.string.page_task_addRelation),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
            Text(
                text = stringResource(R.string.page_task_addRelationDescription),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
            )
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                for (choice in TaskRelationGroup.entries) {
                    FilterChip(
                        selected = choice == group,
                        onClick = { group = choice },
                        label = { Text(stringResource(groupLabel(choice))) },
                    )
                }
            }
            OutlinedTextField(
                value = typed,
                onValueChange = {
                    typed = it
                    onSearch(it)
                },
                singleLine = true,
                label = { Text(stringResource(R.string.page_task_relationSearchPlaceholder)) },
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
            )
            if (candidates.isEmpty()) {
                Text(
                    text =
                        stringResource(
                            if (typed.isBlank()) {
                                R.string.page_task_relationSearchHint
                            } else {
                                R.string.native_search_noMatches
                            },
                        ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
                )
                return@Column
            }
            LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                items(candidates, key = { it.taskLocalId }) { candidate ->
                    ListItem(
                        headlineContent = {
                            Text(
                                text = candidate.title,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        },
                        supportingContent = supportingLine(candidate),
                        modifier = Modifier.clickable { onPick(candidate.taskLocalId, group) },
                    )
                }
            }
        }
    }
}

/**
 * The line under a candidate: where the task lives, and whether it is still open.
 *
 * Two tasks with the same title are told apart by where they sit, and a finished
 * one is worth marking — it is a legitimate thing to link to, and a surprising
 * thing to link to by accident.
 */
@Composable
private fun supportingLine(candidate: TaskRelationCandidate): (@Composable () -> Unit)? {
    val parts =
        listOfNotNull(
            candidate.projectTitle,
            stringResource(R.string.native_search_taskFinished).takeIf { candidate.status != TaskStatus.OPEN },
        )
    if (parts.isEmpty()) return null
    return { Text(parts.joinToString(" · "), style = MaterialTheme.typography.bodySmall) }
}

/** The heading a group is listed under, in the words the web client uses. */
private fun groupLabel(group: TaskRelationGroup): Int =
    when (group) {
        TaskRelationGroup.BLOCKED_BY -> R.string.page_task_relation_blockedBy
        TaskRelationGroup.BLOCKS -> R.string.page_task_relation_blocks
        TaskRelationGroup.RELATED -> R.string.page_task_relation_related
    }

private fun groupIcon(group: TaskRelationGroup) =
    when (group) {
        TaskRelationGroup.BLOCKED_BY -> Icons.Outlined.Lock
        TaskRelationGroup.BLOCKS -> Icons.AutoMirrored.Filled.ArrowForward
        TaskRelationGroup.RELATED -> Icons.Outlined.Link
    }

/**
 * What still holds this task up is the one thing here worth colouring: it is the
 * reason the task cannot be ticked off. A blocker that has been dealt with, and
 * every other kind of link, is ordinary chrome.
 */
@Composable
private fun groupTint(link: TaskRelationRef) =
    if (link.group == TaskRelationGroup.BLOCKED_BY && !link.peerSettled) {
        MaterialTheme.colorScheme.error
    } else {
        MaterialTheme.colorScheme.onSurfaceVariant
    }
