package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import java.time.LocalDate

/**
 * Where the task can go.
 *
 * Ordinarily the inbox leads, because it is the answer for anything not yet
 * decided; the places this device filed work into lately come next, lifted out
 * of the main list so nothing is offered twice, and everything else follows in
 * the order the project list is kept in.
 *
 * While the focus lock is on and the draft is for today, the list is cut by the
 * lock instead: today's projects first, then the inbox, then every other project
 * marked with what choosing it does — it moves the date on to tomorrow — so no
 * entry leads to a dead end.
 */
@Composable
internal fun ProjectPicker(
    state: QuickAddUiState,
    today: LocalDate,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    val searchLabel = stringResource(R.string.dialog_quickAdd_searchProjectsPlaceholder)
    val lockedToday = state.todayLockActive && state.draft.dueDate == today
    val chosen = state.draft.projectLocalId
    Column(modifier = modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 4.dp, end = 16.dp, top = 4.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = { callbacks.onPickingProject(false) }) {
                Icon(
                    Icons.AutoMirrored.Outlined.ArrowBack,
                    contentDescription = stringResource(R.string.native_quickAdd_backToTask),
                )
            }
            OutlinedTextField(
                value = state.projectQuery,
                onValueChange = callbacks.onProjectQuery,
                placeholder = { Text(searchLabel) },
                singleLine = true,
                shape = RoundedCornerShape(24.dp),
                modifier = Modifier.weight(1f).semantics { contentDescription = searchLabel },
            )
        }
        if (lockedToday) PickerFocusNote()
        LazyColumn(
            modifier = Modifier.fillMaxWidth(),
            contentPadding = PaddingValues(bottom = 24.dp),
        ) {
            val inbox: @Composable () -> Unit = {
                PickerRow(
                    title = stringResource(R.string.nav_inbox),
                    icon = Icons.Outlined.Inbox,
                    selected = state.draft.isInbox,
                ) { callbacks.onChooseProject(null) }
            }
            if (lockedToday) {
                val offered = state.recentProjects + state.otherProjects
                val (plan, rest) = offered.partition { it.localId in state.todayPlanIds }
                if (plan.isNotEmpty()) {
                    item { PickerHeading(stringResource(R.string.native_quickAdd_todayPlan)) }
                    items(plan, key = { "plan-${it.localId}" }) { project ->
                        PickerRow(project.title, selected = project.localId == chosen) {
                            callbacks.onChooseProject(project.localId)
                        }
                    }
                }
                item { inbox() }
                if (rest.isNotEmpty()) {
                    item { PickerHeading(stringResource(R.string.native_quickAdd_otherProjects)) }
                    items(rest, key = { "rest-${it.localId}" }) { project ->
                        PickerRow(project.title, selected = project.localId == chosen, notToday = true) {
                            callbacks.onChooseProject(project.localId)
                        }
                    }
                }
            } else {
                item { inbox() }
                if (state.recentProjects.isNotEmpty()) {
                    item { PickerHeading(stringResource(R.string.native_quickAdd_recentProjects)) }
                    items(state.recentProjects, key = { "recent-${it.localId}" }) { project ->
                        PickerRow(project.title, selected = project.localId == chosen) {
                            callbacks.onChooseProject(project.localId)
                        }
                    }
                    item { HorizontalDivider() }
                }
                items(state.otherProjects, key = { it.localId }) { project ->
                    PickerRow(project.title, selected = project.localId == chosen) {
                        callbacks.onChooseProject(project.localId)
                    }
                }
            }
            if (state.recentProjects.isEmpty() && state.otherProjects.isEmpty()) {
                item { PickerHeading(stringResource(R.string.dialog_quickAdd_noMatches)) }
            }
        }
    }
}

/** What the lock does to this list, said once above it. */
@Composable
private fun PickerFocusNote() {
    val tint = MaterialTheme.colorScheme.tertiary
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 4.dp)
                .background(tint.copy(alpha = FOCUS_TINT_ALPHA), RoundedCornerShape(12.dp))
                .padding(horizontal = 12.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Icon(Icons.Outlined.Lock, contentDescription = null, tint = tint, modifier = Modifier.size(16.dp))
        Text(
            text = stringResource(R.string.native_quickAdd_pickerFocus),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun PickerHeading(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 4.dp),
    )
}

@Composable
private fun PickerRow(
    title: String,
    icon: ImageVector = Icons.Outlined.Folder,
    selected: Boolean = false,
    notToday: Boolean = false,
    onClick: () -> Unit,
) {
    val tint = MaterialTheme.colorScheme.tertiary
    ListItem(
        headlineContent = {
            Text(
                text = title,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                color = if (notToday) MaterialTheme.colorScheme.onSurfaceVariant else Color.Unspecified,
            )
        },
        leadingContent = {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
        },
        trailingContent = {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (notToday) {
                    Text(
                        text = stringResource(R.string.native_quickAdd_notToday),
                        style = MaterialTheme.typography.labelMedium,
                        color = tint,
                        modifier =
                            Modifier
                                .background(tint.copy(alpha = FOCUS_TINT_ALPHA), RoundedCornerShape(6.dp))
                                .padding(horizontal = 8.dp, vertical = 3.dp),
                    )
                }
                if (selected) {
                    Icon(Icons.Outlined.Check, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                }
            }
        },
        colors =
            ListItemDefaults.colors(
                containerColor =
                    if (selected) MaterialTheme.colorScheme.surfaceContainerHigh else Color.Transparent,
            ),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).semantics { contentDescription = title },
    )
}
