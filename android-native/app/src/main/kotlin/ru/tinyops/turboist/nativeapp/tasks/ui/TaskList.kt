package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.EventAvailable
import androidx.compose.material.icons.outlined.Inventory2
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.SectionHeading
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.TaskListSection
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import java.time.LocalDate
import java.time.ZoneId

/**
 * What a list says when it holds nothing at all.
 *
 * `null` where a list is built from blocks that name their own emptiness: such a
 * screen keeps its headings whatever it holds, and replacing them with one
 * message would take away the place work is moved *to*.
 */
data class EmptyListText(
    val title: String,
    val description: String,
)

/**
 * What a row can be asked to do, gathered so a screen passes one object instead
 * of six lambdas.
 *
 * The last two are about a picked set rather than a row. [selectionActions] is
 * `null` on a list wired without them, which leaves the picking gesture and the
 * count in place and takes only the buttons away.
 */
data class TaskListCallbacks(
    val onToggleComplete: (Task) -> Unit,
    val onOpen: (Task) -> Unit,
    val onPark: (Task) -> Unit,
    val onPlanForWeek: (Task) -> Unit,
    val onStartSelection: (Task) -> Unit,
    val onSelectToggle: (Task) -> Unit,
    val onClearSelection: () -> Unit,
    val onRefresh: () -> Unit,
    val onSelectAllInSection: (List<Long>) -> Unit = {},
    val selectionActions: SelectionActions? = null,
)

/**
 * A list of tasks, cut into the blocks the screen decided on.
 *
 * The list is a rendering of the replica and never of a response, so there is no
 * error state and no retry here: what the device knows is on screen, whether or
 * not it can reach the server at this moment. Pulling it down does not reload
 * the list — it asks the sync engine to catch up, and the rows change on their
 * own when it does.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskList(
    state: TaskListUiState,
    zone: ZoneId,
    today: LocalDate,
    empty: EmptyListText?,
    callbacks: TaskListCallbacks,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxSize()) {
        if (state.announcement != null) Announcement(state.announcement)
        // The age of the appointments, when they are stale, belongs above the
        // whole list: it qualifies every band on the screen at once, and one line
        // is less noise than the same note repeated under each heading.
        if (state.calendarAsOf != null) CalendarAsOfNote(state.calendarAsOf, zone)
        if (state.selectionMode) {
            SelectionBar(
                count = state.selected.size,
                actions = callbacks.selectionActions,
                onClear = callbacks.onClearSelection,
                modifier = Modifier.background(MaterialTheme.colorScheme.primaryContainer),
            )
        }

        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = callbacks.onRefresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            if (state.isEmpty && empty != null) {
                EmptyList(empty)
                return@PullToRefreshBox
            }
            LazyColumn(modifier = Modifier.fillMaxSize()) {
                for (section in state.sections) {
                    item(key = "heading-" + section.key) {
                        SectionHeader(
                            section = section,
                            selectionMode = state.selectionMode,
                            onSelectAll = {
                                callbacks.onSelectAllInSection(section.rows.map { it.task.localId })
                            },
                        )
                    }
                    if (section.events.isNotEmpty()) {
                        item(key = "events-" + section.key) { CalendarEventBand(section.events, zone) }
                    }
                    if (section.rows.isEmpty() && section.emptyRes != null) {
                        item(key = "empty-" + section.key) { EmptySection(section.emptyRes) }
                    }
                    items(section.rows, key = { "task-" + it.task.localId }) { row ->
                        SwipeableTaskRow(
                            row = row,
                            zone = zone,
                            today = today,
                            selectionMode = state.selectionMode,
                            selected = row.task.localId in state.selected,
                            callbacks = callbacks,
                        )
                    }
                }
            }
        }
    }
}

/**
 * A row with its two gestures.
 *
 * Swiping one way finishes the task; swiping the other moves it between the week
 * and the backlog, in whichever direction the task is not already in. Neither
 * gesture dismisses the row — the row leaves the list only if the change it made
 * takes it out of the query behind it, which is the honest outcome: a completed
 * task disappears from a day view and stays put on a project's.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SwipeableTaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    selectionMode: Boolean,
    selected: Boolean,
    callbacks: TaskListCallbacks,
) {
    val parked = row.task.planState == PlanState.BACKLOG
    // The swipes are unreachable to anyone driving the screen with an assistive
    // reader, so the same two actions are offered as named actions on the row.
    val completeLabel = stringResource(R.string.task_markComplete)
    val planLabel = stringResource(if (parked) R.string.page_nextWeek_planForWeek else R.string.task_actions_toBacklog)
    val swipeState =
        rememberSwipeToDismissBoxState(
            confirmValueChange = { value ->
                when (value) {
                    SwipeToDismissBoxValue.StartToEnd -> callbacks.onToggleComplete(row.task)
                    SwipeToDismissBoxValue.EndToStart ->
                        if (parked) callbacks.onPlanForWeek(row.task) else callbacks.onPark(row.task)

                    SwipeToDismissBoxValue.Settled -> Unit
                }
                false
            },
        )
    SwipeToDismissBox(
        state = swipeState,
        modifier =
            Modifier.semantics {
                customActions =
                    listOf(
                        CustomAccessibilityAction(completeLabel) {
                            callbacks.onToggleComplete(row.task)
                            true
                        },
                        CustomAccessibilityAction(planLabel) {
                            if (parked) callbacks.onPlanForWeek(row.task) else callbacks.onPark(row.task)
                            true
                        },
                    )
            },
        backgroundContent = { SwipeBackground(parked) },
        content = {
            TaskRow(
                row = row,
                zone = zone,
                today = today,
                selectionMode = selectionMode,
                selected = selected,
                onToggleComplete = { callbacks.onToggleComplete(row.task) },
                onOpen = { callbacks.onOpen(row.task) },
                onSelectToggle = { callbacks.onSelectToggle(row.task) },
                onStartSelection = { callbacks.onStartSelection(row.task) },
                modifier = Modifier.background(MaterialTheme.colorScheme.surface),
            )
        },
    )
}

/**
 * What shows behind a row being swiped: the action each direction stands for.
 *
 * The two icons are described to no one: they are the feedback of a gesture, and
 * they exist only while that gesture is under way. A reader that cannot make the
 * gesture is offered the same two actions on the row itself instead, which is
 * where they can actually be taken.
 */
@Composable
private fun SwipeBackground(parked: Boolean) {
    Row(
        modifier =
            Modifier
                .fillMaxSize()
                .background(MaterialTheme.colorScheme.surfaceVariant)
                .padding(horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Icon(
            imageVector = Icons.Outlined.CheckCircle,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Icon(
            imageVector = if (parked) Icons.Outlined.EventAvailable else Icons.Outlined.Inventory2,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * The heading above a block, or nothing at all for a list that has no blocks.
 *
 * While tasks are being picked the heading also offers the whole block at once.
 * Picking twenty rows one at a time is the gesture selection mode exists to
 * avoid, and the heading is the only place on screen that knows which rows make
 * up a block.
 */
@Composable
private fun SectionHeader(
    section: TaskListSection,
    selectionMode: Boolean = false,
    onSelectAll: () -> Unit = {},
) {
    val heading = section.heading
    if (heading is SectionHeading.None) {
        if (selectionMode && section.rows.isNotEmpty()) SelectAllRow(onSelectAll)
        return
    }
    val overdue = heading is SectionHeading.Overdue
    val emphasised = overdue || (heading is SectionHeading.Phase && heading.active)
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 12.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(
            text = headingText(heading),
            style = MaterialTheme.typography.titleSmall,
            color =
                when {
                    overdue -> MaterialTheme.colorScheme.error
                    emphasised -> MaterialTheme.colorScheme.onSurface
                    else -> MaterialTheme.colorScheme.onSurfaceVariant
                },
        )
        Text(
            // Everything the block holds, appointments included: the number
            // answers "how much is in here", and a heading with two meetings
            // under it must not read as a zero.
            text = (section.rows.size + section.events.size).toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        if (selectionMode && section.rows.isNotEmpty()) {
            TextButton(onClick = onSelectAll) { Text(stringResource(R.string.selection_bar_selectAll)) }
        }
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

/** The whole-block gesture on a list whose blocks carry no heading of their own. */
@Composable
private fun SelectAllRow(onSelectAll: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp),
        horizontalArrangement = Arrangement.End,
    ) {
        TextButton(onClick = onSelectAll) { Text(stringResource(R.string.selection_bar_selectAll)) }
    }
}

@Composable
private fun headingText(heading: SectionHeading): String =
    when (heading) {
        is SectionHeading.None -> ""
        is SectionHeading.Overdue -> stringResource(R.string.page_today_overdueTitle)
        is SectionHeading.Phase -> stringResource(dayPartLabel(heading.part))
        is SectionHeading.Day -> dayHeadingLabel(heading.day, heading.relative)
        is SectionHeading.Named -> stringResource(heading.titleRes)
    }

/**
 * The wording for a phase of the day. A phase this build does not recognise
 * reads as "any time", which is where the grouping already put its tasks.
 *
 * Shared with the screens that let a phase be chosen, so a heading and a picker
 * cannot end up calling the same phase two different things.
 */
fun dayPartLabel(part: DayPart): Int =
    when (part) {
        DayPart.MORNING -> R.string.task_dayPart_morning
        DayPart.AFTERNOON -> R.string.task_dayPart_afternoon
        DayPart.EVENING -> R.string.task_dayPart_evening
        DayPart.NONE, DayPart.UNKNOWN -> R.string.task_dayPart_anytime
    }

/** The user's own message, shown above the day it was written for. */
@Composable
private fun Announcement(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSecondaryContainer,
        modifier =
            Modifier
                .fillMaxWidth()
                .background(MaterialTheme.colorScheme.secondaryContainer)
                .padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/**
 * What a list says when it holds nothing.
 *
 * Phrased as a state of the work rather than as a failure, because on a device
 * that reads its own replica an empty list means there is nothing to do — not
 * that something went wrong fetching it.
 */
@Composable
internal fun EmptyList(empty: EmptyListText) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(32.dp),
        ) {
            Text(
                text = empty.title,
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
                textAlign = TextAlign.Center,
            )
            Text(
                text = empty.description,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
    }
}

/** What a block of a list says while it is waiting for its first row. */
@Composable
private fun EmptySection(
    @StringRes textRes: Int,
) {
    Text(
        text = stringResource(textRes),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
    )
}
