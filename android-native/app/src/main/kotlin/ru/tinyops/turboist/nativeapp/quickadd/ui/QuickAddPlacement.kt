package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.layout
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import java.time.LocalDate

/**
 * Where the task goes and when it is for, in one card.
 *
 * The two are drawn together because the Today focus lock ties them together: a
 * task for today may only go into a project from today's plan. While the lock is
 * on, the card says so in its heading, leads with today's projects, marks the
 * "today" choice with a padlock, and — when the draft as it stands would be
 * refused — explains why right there and offers the two ways out, instead of
 * leaving the user to find out from a refusal after saving.
 */
@Composable
internal fun PlacementCard(
    state: QuickAddUiState,
    today: LocalDate,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    val conflict = state.todayLockConflict
    val outline = if (conflict) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.outlineVariant
    val name = state.projectTitle ?: stringResource(R.string.nav_inbox)
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        border = BorderStroke(1.dp, outline),
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    ) {
        Column {
            if (state.todayLockActive) FocusStrip()
            Column(
                modifier = Modifier.fillMaxWidth().padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FieldCaption(stringResource(R.string.native_quickAdd_project))
                ProjectButton(
                    title = name,
                    inbox = state.projectTitle == null,
                    onClick = { callbacks.onPickingProject(true) },
                )
                QuickProjects(state = state, onChoose = callbacks.onChooseProject)
            }
            HorizontalDivider(
                color = MaterialTheme.colorScheme.outlineVariant,
                modifier = Modifier.padding(horizontal = 14.dp),
            )
            Column(
                modifier = Modifier.fillMaxWidth().padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FieldCaption(stringResource(R.string.native_quickAdd_due))
                // A date is a plan, and the inbox holds what has not been planned
                // yet, so scheduling is offered only once the task has a home. The
                // same rule hides these controls in the web client.
                if (state.draft.isInbox) {
                    Text(
                        text = stringResource(R.string.native_quickAdd_inboxNoDue),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    DueChoices(
                        dueDate = state.draft.dueDate,
                        today = today,
                        locked = state.todayLockActive,
                        conflict = conflict,
                        onPick = callbacks.onDueDate,
                    )
                }
                if (state.dueShiftedByLock && !conflict) ShiftNotice(name)
                if (conflict) {
                    ConflictNotice(
                        projectTitle = name,
                        onTomorrow = { callbacks.onDueDate(today.plusDays(1)) },
                        onPickProject = { callbacks.onPickingProject(true) },
                    )
                }
            }
        }
    }
}

/** The card's heading while the focus lock is on: what it is and until when. */
@Composable
private fun FocusStrip() {
    val tint = MaterialTheme.colorScheme.tertiary
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .background(tint.copy(alpha = FOCUS_TINT_ALPHA))
                .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(Icons.Outlined.Lock, contentDescription = null, tint = tint, modifier = Modifier.size(16.dp))
        val title = stringResource(R.string.native_quickAdd_focusTitle)
        val body = stringResource(R.string.native_quickAdd_focusBody)
        Text(
            text =
                buildAnnotatedString {
                    withStyle(SpanStyle(color = tint, fontWeight = FontWeight.Medium)) { append(title) }
                    append(" · ")
                    append(body)
                },
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
internal fun FieldCaption(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** Where the task is going, and the way to change it. */
@Composable
private fun ProjectButton(
    title: String,
    inbox: Boolean,
    onClick: () -> Unit,
) {
    val description = stringResource(R.string.native_quickAdd_changeProject, title)
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = description },
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                imageVector = if (inbox) Icons.Outlined.Inbox else Icons.Outlined.Folder,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(20.dp),
            )
            Text(
                text = title,
                style = MaterialTheme.typography.titleMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Icon(
                Icons.Outlined.ExpandMore,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * The places one tap away, on a single line that scrolls sideways rather than
 * wrapping into rows: today's projects when they are what the sheet leads with,
 * otherwise the places this device filed work into lately, and the inbox.
 */
@Composable
private fun QuickProjects(
    state: QuickAddUiState,
    onChoose: (Long?) -> Unit,
) {
    val leadWithToday = state.todayProjects.isNotEmpty()
    val projects: List<Project> = if (leadWithToday) state.todayProjects else state.recentProjects.take(RECENT_LIMIT)
    if (projects.isEmpty()) return
    val chosen = state.draft.projectLocalId
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        FieldCaption(
            stringResource(
                if (leadWithToday) R.string.native_quickAdd_todayPlan else R.string.native_quickAdd_recentProjects,
            ),
        )
        Row(
            // The row runs to the card's edges and scrolls under them, so a chip
            // that does not fit reads as "more this way" rather than as cut off.
            modifier =
                Modifier
                    .bleed(CARD_PADDING)
                    .horizontalScroll(rememberScrollState())
                    .padding(horizontal = CARD_PADDING),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (project in projects) {
                ProjectChip(title = project.title, selected = project.localId == chosen) { onChoose(project.localId) }
            }
            if (!leadWithToday) {
                ProjectChip(
                    title = stringResource(R.string.nav_inbox),
                    selected = state.draft.isInbox,
                ) { onChoose(null) }
            }
        }
    }
}

@Composable
private fun ProjectChip(
    title: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(title, maxLines = 1) },
        modifier = Modifier.semantics { contentDescription = title },
    )
}

/** Says that choosing a project outside today's plan moved the date on to tomorrow. */
@Composable
private fun ShiftNotice(projectTitle: String) {
    Row(
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            Icons.Outlined.Info,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.tertiary,
            modifier = Modifier.size(16.dp).padding(top = 1.dp),
        )
        Text(
            text = stringResource(R.string.native_quickAdd_dueShifted, projectTitle),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * Why the draft as it stands would be refused, with the two ways out: another
 * day, or a project from today's plan. Saving waits until one of them is taken.
 */
@Composable
private fun ConflictNotice(
    projectTitle: String,
    onTomorrow: () -> Unit,
    onPickProject: () -> Unit,
) {
    val tint = MaterialTheme.colorScheme.tertiary
    Column(
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(top = 4.dp)
                .background(tint.copy(alpha = CONFLICT_FILL_ALPHA), RoundedCornerShape(12.dp))
                .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.Top) {
            Icon(Icons.Outlined.Lock, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = stringResource(R.string.native_quickAdd_conflictTitle, projectTitle),
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSurface,
                )
                Text(
                    text = stringResource(R.string.native_quickAdd_conflictBody),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(
                onClick = onTomorrow,
                border = BorderStroke(1.dp, tint),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = tint),
                contentPadding = PaddingValues(horizontal = 12.dp),
                modifier = Modifier.weight(1f),
            ) { Text(stringResource(R.string.native_quickAdd_conflictTomorrow), maxLines = 1) }
            Button(
                onClick = onPickProject,
                colors =
                    ButtonDefaults.buttonColors(
                        containerColor = tint,
                        contentColor = MaterialTheme.colorScheme.onTertiary,
                    ),
                contentPadding = PaddingValues(horizontal = 12.dp),
                modifier = Modifier.weight(WIDE_BUTTON_WEIGHT),
            ) { Text(stringResource(R.string.native_quickAdd_conflictPickProject), maxLines = 1) }
        }
    }
}

/** The card's inner margin. */
private val CARD_PADDING = 14.dp

/** Widens a child past its parent's padding by [horizontal] on either side. */
private fun Modifier.bleed(horizontal: Dp): Modifier =
    layout { measurable, constraints ->
        val extra = (horizontal * 2).roundToPx()
        val placeable =
            measurable.measure(
                constraints.copy(minWidth = constraints.maxWidth + extra, maxWidth = constraints.maxWidth + extra),
            )
        layout(constraints.maxWidth, placeable.height) { placeable.place(-extra / 2, 0) }
    }

/** How much of the lock's colour the focus heading is washed with. */
internal const val FOCUS_TINT_ALPHA = 0.09f

/** And the clash, a little stronger. */
internal const val CONFLICT_FILL_ALPHA = 0.14f

/** The second way out carries the longer wording. */
private const val WIDE_BUTTON_WEIGHT = 1.3f

/** How many recent places the card offers before the full list is a tap away. */
private const val RECENT_LIMIT = 6
