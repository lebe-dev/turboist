package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.NorthEast
import androidx.compose.material.icons.filled.SubdirectoryArrowRight
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.layout
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.toSize
import ru.tinyops.turboist.core.model.view.DependencyDropRefusal
import ru.tinyops.turboist.core.model.view.NestDropRefusal
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import kotlin.math.roundToInt

/**
 * The one panel a drag opens, holding every action the drop can mean.
 *
 * It floats over the page rather than being laid out in it: appearing must never
 * push the rows out from under the finger, and it has to stay reachable while the
 * finger travels to it — so it is anchored to a row, not to the finger itself.
 * The list leaves no trace of what a drop could do, and the old edge-versus-middle
 * rule was invisible on a phone; an icon with a name under it is not.
 *
 * The row being pointed at is chosen by dragging over it, is never covered by the
 * panel, and is kept as the target while the finger moves up onto an action. An
 * action the target refuses shows greyed and says why in the caption.
 */
@Composable
internal fun SubtaskDragPanel(
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
    rows: List<TaskListRow>,
    container: Rect,
    modifier: Modifier = Modifier,
) {
    DisposableEffect(drag) {
        onDispose {
            drag.placePanel(null)
            SubtaskDropAction.entries.forEach { drag.placeAction(it, null) }
        }
    }
    val dragged = drag.draggedLocalId ?: return
    val anchor = drag.anchor ?: return
    val draggedRow = rows.firstOrNull { it.task.localId == dragged }
    val target = drag.hoverLocalId?.let { id -> rows.firstOrNull { it.task.localId == id } }
    val hovered = drag.hoveredAction
    val gap = with(LocalDensity.current) { PANEL_GAP.roundToPx() }.toFloat()
    Surface(
        shape = RoundedCornerShape(16.dp),
        tonalElevation = 3.dp,
        shadowElevation = 6.dp,
        modifier =
            modifier.fillMaxWidth().layout { measurable, constraints ->
                val placeable = measurable.measure(constraints.copy(minHeight = 0))
                val top = panelPlacement(anchor, container, placeable.height.toFloat(), gap)
                // The card's own height is what the finger can land on; the area
                // this layout claims is the whole page, which is only how the
                // panel gets to float over it.
                drag.placePanel(Rect(container.left, top, container.right, top + placeable.height))
                layout(constraints.maxWidth, constraints.maxHeight) {
                    placeable.place(0, (top - container.top).roundToInt())
                }
            },
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            val refused =
                hovered != null && target != null &&
                    isRefused(hovered, dependencies, dragged, draggedRow, target)
            Text(
                text = panelCaption(dependencies, dragged, draggedRow, target, hovered),
                style = MaterialTheme.typography.labelMedium,
                color =
                    if (refused) {
                        MaterialTheme.colorScheme.error
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(horizontal = 8.dp),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                for (action in SubtaskDropAction.entries) {
                    PanelAction(action, drag, dependencies, dragged, draggedRow, target, Modifier.weight(1f))
                }
            }
        }
    }
}

/**
 * Where the drag panel opens: just above the row it belongs to.
 *
 * Above rather than below, because a finger travels down a list far more often
 * than up: with the panel underneath, the very first row a drag reached would
 * open the panel over the next one and block the way down. Above, the list keeps
 * going, and travelling up into the panel is the commit.
 */
internal fun panelPlacement(
    anchor: Rect,
    container: Rect,
    height: Float,
    gap: Float,
): Float {
    val above = anchor.top - gap - height
    if (above >= container.top) return above
    val below = anchor.bottom + gap
    if (below + height <= container.bottom) return below
    return container.top + gap
}

/** One action of the panel: an icon with its name under it, dimmed while it cannot be used. */
@Composable
private fun PanelAction(
    action: SubtaskDropAction,
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    draggedRow: TaskListRow?,
    target: TaskListRow?,
    modifier: Modifier = Modifier,
) {
    val usable = actionUsable(action, dependencies, draggedLocalId, draggedRow, target)
    val active = drag.hoveredAction == action
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier =
            modifier
                .onGloballyPositioned { drag.placeAction(action, Rect(it.positionInRoot(), it.size.toSize())) }
                .alpha(if (usable) 1f else DISABLED_ALPHA)
                .clip(RoundedCornerShape(12.dp))
                .background(
                    if (active && usable) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent,
                ).padding(vertical = 8.dp, horizontal = 4.dp),
    ) {
        Icon(
            imageVector = actionIcon(action),
            contentDescription = null,
            tint = actionTint(action, active && usable),
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = stringResource(actionLabel(action)),
            style = MaterialTheme.typography.labelSmall,
            color =
                if (active && usable) {
                    MaterialTheme.colorScheme.onSecondaryContainer
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            textAlign = TextAlign.Center,
            maxLines = 2,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

/** Whether [action] can be taken on [target] as things stand. */
private fun actionUsable(
    action: SubtaskDropAction,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    draggedRow: TaskListRow?,
    target: TaskListRow?,
): Boolean =
    when (action) {
        SubtaskDropAction.DETACH -> canDetach(dependencies, draggedRow)
        SubtaskDropAction.NEST ->
            target != null &&
                !dependencies.nestNoop(draggedLocalId, target.task.localId) &&
                dependencies.nestRefusal(draggedLocalId, target.task.localId) == null
        SubtaskDropAction.DEPENDENCY ->
            target != null && dependencies.dependencyRefusal(draggedLocalId, target.task.localId) == null
    }

/**
 * Whether the carried row has a parent to leave — the screen offering detach at
 * all is what [SubtaskDependencies.onDetachDrop] says, and a top-level row has
 * nowhere to go either way.
 */
private fun canDetach(
    dependencies: SubtaskDependencies,
    draggedRow: TaskListRow?,
): Boolean = dependencies.onDetachDrop != null && draggedRow?.task?.parentLocalId != null

/**
 * What the panel says about the gesture as it stands: the refusal that would meet
 * the action under the finger, what that action would do, which task is being
 * pointed at, or — with nothing in the way yet — what to do at all.
 */
@Composable
private fun panelCaption(
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    draggedRow: TaskListRow?,
    target: TaskListRow?,
    hovered: SubtaskDropAction?,
): String {
    if (hovered == null) {
        return target?.task?.title ?: stringResource(R.string.native_task_dragPanel_hint)
    }
    if (hovered == SubtaskDropAction.DETACH) {
        return stringResource(R.string.page_task_detachDrag_hint)
    }
    if (target == null) return stringResource(R.string.native_task_dragPanel_hint)
    if (isRefused(hovered, dependencies, draggedLocalId, draggedRow, target)) {
        return refusalText(hovered, dependencies, draggedLocalId, target)
    }
    return when (hovered) {
        SubtaskDropAction.NEST -> stringResource(R.string.page_task_nestDrag_willNest, target.task.title)
        else -> stringResource(R.string.page_task_dependencyDrag_willDepend, target.task.title)
    }
}

/** The sentence for an action the target refuses. */
@Composable
private fun refusalText(
    action: SubtaskDropAction,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    target: TaskListRow,
): String {
    val title = target.task.title
    return when (action) {
        SubtaskDropAction.NEST ->
            when (dependencies.nestRefusal(draggedLocalId, target.task.localId)) {
                null -> stringResource(R.string.page_task_nestDrag_willNest, title)
                NestDropRefusal.DESCENDANT -> stringResource(R.string.page_task_nestDrag_refusedDescendant)
                NestDropRefusal.COMPLETED -> stringResource(R.string.page_task_nestDrag_refusedCompleted, title)
            }
        SubtaskDropAction.DEPENDENCY ->
            when (dependencies.dependencyRefusal(draggedLocalId, target.task.localId)) {
                null -> stringResource(R.string.page_task_dependencyDrag_willDepend, title)
                DependencyDropRefusal.ANCESTOR -> stringResource(R.string.page_task_dependencyDrag_refusedAncestor)
                DependencyDropRefusal.DESCENDANT ->
                    stringResource(R.string.page_task_dependencyDrag_refusedDescendant)
                DependencyDropRefusal.COMPLETED ->
                    stringResource(R.string.page_task_dependencyDrag_refusedCompleted, title)
                DependencyDropRefusal.EXISTS -> stringResource(R.string.page_task_dependencyDrag_refusedExists, title)
                DependencyDropRefusal.CYCLE -> stringResource(R.string.page_task_dependencyDrag_refusedCycle, title)
            }
        SubtaskDropAction.DETACH -> stringResource(R.string.page_task_detachDrag_hint)
    }
}

/**
 * The marks a row wears during a drag: outlined while it is the target, in the
 * error colour when what is about to happen would be refused, with the chosen
 * action on a badge. A nest onto the row's own parent shows nothing, the same way
 * hovering the dragged row itself does — it would promise an effect there is none
 * of.
 */
@Composable
fun BoxScope.SubtaskDragMarks(
    row: TaskListRow,
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
) {
    val dragged = drag.draggedLocalId ?: return
    if (drag.hoverLocalId != row.task.localId) return
    val action = drag.hoveredAction
    if (action == null && dependencies.nestNoop(dragged, row.task.localId)) return
    // Lifting the row out is about the carried row rather than this one, so it
    // is never refused *at* a target.
    val refused =
        action != null && action != SubtaskDropAction.DETACH &&
            isRefused(action, dependencies, dragged, draggedRow = null, target = row)
    val color = if (refused) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary
    val onColor = if (refused) MaterialTheme.colorScheme.onError else Color.White
    Box(
        modifier =
            Modifier
                .matchParentSize()
                .border(width = 2.dp, color = color, shape = RoundedCornerShape(8.dp)),
    )
    if (action == null) return
    Box(
        contentAlignment = Alignment.Center,
        modifier =
            Modifier
                .align(Alignment.CenterEnd)
                .padding(end = 12.dp)
                .size(28.dp)
                .background(color, CircleShape),
    ) {
        Icon(
            imageVector = if (refused) Icons.Filled.Block else actionIcon(action),
            contentDescription = null,
            tint = onColor,
            modifier = Modifier.size(16.dp),
        )
    }
}

/** Fades the row that is being carried. */
fun Modifier.subtaskDragFade(
    row: TaskListRow,
    drag: SubtaskDragState,
): Modifier = if (drag.draggedLocalId == row.task.localId) alpha(DRAGGED_ALPHA) else this

/** Whether [action] on [target] would be refused, or would change nothing. */
private fun isRefused(
    action: SubtaskDropAction,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    draggedRow: TaskListRow?,
    target: TaskListRow,
): Boolean =
    when (action) {
        SubtaskDropAction.DETACH -> !canDetach(dependencies, draggedRow)
        SubtaskDropAction.NEST ->
            dependencies.nestNoop(draggedLocalId, target.task.localId) ||
                dependencies.nestRefusal(draggedLocalId, target.task.localId) != null
        SubtaskDropAction.DEPENDENCY ->
            dependencies.dependencyRefusal(draggedLocalId, target.task.localId) != null
    }

@Composable
private fun actionLabel(action: SubtaskDropAction): Int =
    when (action) {
        SubtaskDropAction.NEST -> R.string.native_task_dragAction_nest
        SubtaskDropAction.DEPENDENCY -> R.string.native_task_dragAction_depend
        SubtaskDropAction.DETACH -> R.string.native_task_dragAction_detach
    }

private fun actionIcon(action: SubtaskDropAction): ImageVector =
    when (action) {
        SubtaskDropAction.NEST -> Icons.Filled.SubdirectoryArrowRight
        SubtaskDropAction.DEPENDENCY -> Icons.Filled.Lock
        SubtaskDropAction.DETACH -> Icons.Filled.NorthEast
    }

@Composable
private fun actionTint(
    action: SubtaskDropAction,
    active: Boolean,
): Color =
    when {
        active -> MaterialTheme.colorScheme.onSecondaryContainer
        action == SubtaskDropAction.NEST -> NEST_COLOR
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }

// The same violet accent the web client uses for a nest drop, distinct from the
// primary blue a dependency drop uses — the two gestures must never look alike.
private val NEST_COLOR = Color(0xFF8B5CF6)

private const val DRAGGED_ALPHA = 0.4f

/** How far an action the target refuses is dimmed: visible, not offered. */
private const val DISABLED_ALPHA = 0.38f

/** The gap between the row a drag belongs to and the panel opened beside it. */
private val PANEL_GAP = 8.dp
