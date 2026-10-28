package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.AnchoredDraggableState
import androidx.compose.foundation.gestures.DraggableAnchors
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.anchoredDraggable
import androidx.compose.foundation.gestures.animateTo
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.EventAvailable
import androidx.compose.material.icons.outlined.Inventory2
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.shell.LocalShellChrome
import ru.tinyops.turboist.nativeapp.tasks.SectionHeading
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.TaskListSection
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import java.time.LocalDate
import java.time.ZoneId
import kotlin.math.roundToInt

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
    // While tasks are picked the toolbar owns the bottom edge, so the shell's
    // capture button and navigation bar step aside for exactly that long.
    val chrome = LocalShellChrome.current
    if (chrome != null) {
        DisposableEffect(chrome, state.selectionMode) {
            chrome.selectionActive = state.selectionMode
            onDispose { chrome.selectionActive = false }
        }
    }
    val actions = callbacks.selectionActions
    val toolbarShown = state.selectionMode && actions != null

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            if (state.announcement != null) Announcement(state.announcement)
            // The age of the appointments, when they are stale, belongs above the
            // whole list: it qualifies every band on the screen at once, and one
            // line is less noise than the same note repeated under each heading.
            if (state.calendarAsOf != null) CalendarAsOfNote(state.calendarAsOf, zone)
            if (state.selectionMode) {
                SelectionHeader(count = state.selected.size, onClear = callbacks.onClearSelection)
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
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding =
                        PaddingValues(
                            start = LIST_GUTTER,
                            end = LIST_GUTTER,
                            // The last row has to be able to scroll clear of
                            // whatever floats over the bottom of the list.
                            bottom = if (toolbarShown) TOOLBAR_CLEARANCE else FAB_CLEARANCE,
                        ),
                ) {
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
                        itemsIndexed(section.rows, key = { _, row -> "task-" + row.task.localId }) { index, row ->
                            SwipeableTaskRow(
                                row = row,
                                zone = zone,
                                today = today,
                                selectionMode = state.selectionMode,
                                selected = row.task.localId in state.selected,
                                callbacks = callbacks,
                                shape = segmentShape(index, section.rows.size),
                                modifier = Modifier.padding(top = if (index == 0) 0.dp else SEGMENT_GAP),
                            )
                        }
                    }
                }
            }
        }
        if (toolbarShown && actions != null) {
            SelectionToolbar(
                count = state.selected.size,
                actions = actions,
                modifier =
                    Modifier
                        .align(Alignment.BottomCenter)
                        .padding(horizontal = 12.dp, vertical = 16.dp),
            )
        }
    }
}

/**
 * The shape of a row inside its block: the block reads as one card, so only its
 * outer corners are fully rounded and the seams between rows are barely cut.
 */
internal fun segmentShape(
    index: Int,
    count: Int,
): Shape {
    val top = if (index == 0) OUTER_CORNER else INNER_CORNER
    val bottom = if (index == count - 1) OUTER_CORNER else INNER_CORNER
    return RoundedCornerShape(topStart = top, topEnd = top, bottomStart = bottom, bottomEnd = bottom)
}

/** Where a row rests after it has been swiped. */
private enum class RowSwipe { COMPLETE, SETTLED, REVEALED }

/** One action a swiped row uncovers. */
private data class RowAction(
    val icon: ImageVector,
    @param:StringRes val labelRes: Int,
    val run: () -> Unit,
)

/**
 * A row with its two gestures.
 *
 * Swiping towards the end finishes the task. Swiping towards the start only
 * uncovers the actions behind the row — the week or the backlog, whichever the
 * task is not already in, and picking it — and the row stays open until one of
 * them is tapped or the row is swiped or tapped shut. Taking the action on the
 * swipe itself made it far too easy to park a task by accident while scrolling.
 *
 * Neither gesture dismisses the row: it leaves the list only if the change takes
 * it out of the query behind it, which is the honest outcome — a completed task
 * disappears from a day view and stays put on a project's.
 */
@Composable
private fun SwipeableTaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    selectionMode: Boolean,
    selected: Boolean,
    callbacks: TaskListCallbacks,
    shape: Shape,
    modifier: Modifier = Modifier,
) {
    val task = row.task
    val scope = rememberCoroutineScope()
    val density = LocalDensity.current
    val swipe = remember { AnchoredDraggableState(RowSwipe.SETTLED) }
    val close: () -> Unit = { scope.launch { swipe.animateTo(RowSwipe.SETTLED) } }
    val actions =
        buildList {
            if (task.planState != PlanState.WEEK) {
                add(
                    RowAction(
                        Icons.Outlined.EventAvailable,
                        R.string.native_swipe_week,
                    ) { callbacks.onPlanForWeek(task) },
                )
            }
            if (task.planState != PlanState.BACKLOG) {
                add(RowAction(Icons.Outlined.Inventory2, R.string.native_swipe_backlog) { callbacks.onPark(task) })
            }
            add(RowAction(Icons.Outlined.Checklist, R.string.native_swipe_select) { callbacks.onStartSelection(task) })
        }
    val panelWidth = ACTION_WIDTH * actions.size
    val revealing by remember { derivedStateOf { swipe.offset.let { !it.isNaN() && it < 0f } } }
    val completing by remember { derivedStateOf { swipe.offset.let { !it.isNaN() && it > 0f } } }

    // Ticking the task off is the one swipe that acts on its own; the row springs
    // back as soon as it has.
    LaunchedEffect(swipe.settledValue) {
        if (swipe.settledValue != RowSwipe.COMPLETE) return@LaunchedEffect
        callbacks.onToggleComplete(task)
        swipe.animateTo(RowSwipe.SETTLED)
    }
    // Picking starts with every row shut: an open one would hold actions that
    // are not what a tap means any more.
    LaunchedEffect(selectionMode) {
        if (selectionMode) swipe.animateTo(RowSwipe.SETTLED)
    }

    // The swipes are unreachable to anyone driving the screen with an assistive
    // reader, so the same actions are offered as named actions on the row.
    val completeLabel = stringResource(R.string.task_markComplete)
    val actionLabels = actions.map { stringResource(it.labelRes) }
    Box(
        modifier =
            modifier
                .clip(shape)
                .onSizeChanged { size ->
                    val panel = with(density) { panelWidth.toPx() }
                    swipe.updateAnchors(
                        DraggableAnchors {
                            RowSwipe.COMPLETE at size.width * COMPLETE_TRAVEL
                            RowSwipe.SETTLED at 0f
                            RowSwipe.REVEALED at -panel
                        },
                    )
                }.semantics {
                    customActions =
                        listOf(
                            CustomAccessibilityAction(completeLabel) {
                                callbacks.onToggleComplete(task)
                                true
                            },
                        ) +
                        actions.mapIndexed { index, action ->
                            CustomAccessibilityAction(actionLabels[index]) {
                                action.run()
                                true
                            }
                        }
                },
    ) {
        if (completing) CompleteBackground(modifier = Modifier.matchParentSize())
        if (revealing) {
            RowActionPanel(
                actions = actions,
                onAction = { action ->
                    close()
                    action.run()
                },
                modifier = Modifier.matchParentSize(),
            )
        }
        TaskRow(
            row = row,
            zone = zone,
            today = today,
            selectionMode = selectionMode,
            selected = selected,
            onToggleComplete = { callbacks.onToggleComplete(task) },
            // A tap on an open row shuts it: opening the task from under the
            // actions would leave them hanging there on the way back.
            onOpen = { if (swipe.currentValue == RowSwipe.SETTLED) callbacks.onOpen(task) else close() },
            onSelectToggle = { callbacks.onSelectToggle(task) },
            onStartSelection = { callbacks.onStartSelection(task) },
            modifier =
                Modifier
                    .offset { IntOffset(swipe.offset.takeUnless { it.isNaN() }?.roundToInt() ?: 0, 0) }
                    .anchoredDraggable(swipe, Orientation.Horizontal, enabled = !selectionMode)
                    .background(MaterialTheme.colorScheme.surfaceContainerLow),
        )
    }
}

/**
 * What shows behind a row being swiped towards the end: the tick it stands for.
 *
 * The icon is described to no one: it is the feedback of a gesture and exists
 * only while that gesture is under way. A reader that cannot make the gesture is
 * offered the same action on the row itself instead.
 */
@Composable
private fun CompleteBackground(modifier: Modifier = Modifier) {
    Row(
        modifier =
            modifier
                .background(MaterialTheme.colorScheme.surfaceContainerHigh)
                .padding(horizontal = 20.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = Icons.Outlined.CheckCircle,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** The actions a row uncovers when it is swiped towards the start, each named under its icon. */
@Composable
private fun RowActionPanel(
    actions: List<RowAction>,
    onAction: (RowAction) -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier.background(MaterialTheme.colorScheme.surfaceContainerHigh),
        horizontalArrangement = Arrangement.End,
    ) {
        for (action in actions) {
            Column(
                modifier =
                    Modifier
                        .width(ACTION_WIDTH)
                        .fillMaxHeight()
                        .clickable { onAction(action) }
                        .padding(horizontal = 4.dp, vertical = 8.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center,
            ) {
                Icon(
                    imageVector = action.icon,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = stringResource(action.labelRes),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    maxLines = 2,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
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
    val emphasised = heading is SectionHeading.Phase && heading.active
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 8.dp, end = 0.dp, top = 20.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = headingText(heading),
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.SemiBold,
            color =
                when {
                    overdue -> MaterialTheme.colorScheme.error
                    emphasised -> MaterialTheme.colorScheme.primary
                    else -> MaterialTheme.colorScheme.onSurfaceVariant
                },
        )
        if (heading is SectionHeading.Phase) {
            // The parts of a day are headed by their name alone: on the day
            // views a number beside each of them was noise, not information.
            Spacer(modifier = Modifier.weight(1f))
        } else {
            Text(
                // Everything the block holds, appointments included: the number
                // answers "how much is in here", and a heading with two meetings
                // under it must not read as a zero.
                text = (section.rows.size + section.events.size).toString(),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.weight(1f),
            )
        }
        if (selectionMode && section.rows.isNotEmpty()) {
            TextButton(onClick = onSelectAll) { Text(stringResource(R.string.selection_bar_selectAll)) }
        }
    }
}

/** The whole-block gesture on a list whose blocks carry no heading of their own. */
@Composable
private fun SelectAllRow(onSelectAll: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
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
                .padding(horizontal = LIST_GUTTER, vertical = 4.dp)
                .background(MaterialTheme.colorScheme.secondaryContainer, RoundedCornerShape(OUTER_CORNER))
                .padding(horizontal = 16.dp, vertical = 12.dp),
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
        modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 12.dp),
    )
}

/** How far the cards of a list sit in from the screen's edges. */
private val LIST_GUTTER = 12.dp

/** The corner a block of rows is rounded with on the outside. */
private val OUTER_CORNER = 20.dp

/** The corner cut into the seam between two rows of one block. */
private val INNER_CORNER = 4.dp

/** The gap between two rows of one block. */
private val SEGMENT_GAP = 2.dp

/** Room under the last row for the capture button to float over nothing. */
private val FAB_CLEARANCE = 96.dp

/** The width of one action a swiped row uncovers. */
private val ACTION_WIDTH = 76.dp

/** How far, as a share of its width, a row travels towards the end to be ticked off. */
private const val COMPLETE_TRAVEL = 0.4f

/** Room under the last row for the selection toolbar. */
private val TOOLBAR_CLEARANCE = 112.dp
