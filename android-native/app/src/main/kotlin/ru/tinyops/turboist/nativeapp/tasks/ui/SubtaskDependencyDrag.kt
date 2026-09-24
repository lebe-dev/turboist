package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.toSize
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.DependencyDropRefusal
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow

/**
 * What the subtask rows need to take part in "drop one onto another to make it
 * wait for it".
 *
 * [refusal] is asked on every hover, so it has to be a cheap read of the
 * replica — it is: the screen state already holds the tree and the edges.
 */
class SubtaskDependencies(
    val enabled: Boolean,
    val refusal: (draggedLocalId: Long, targetLocalId: Long) -> DependencyDropRefusal?,
    val onDrop: (draggedLocalId: Long, targetLocalId: Long) -> Unit,
) {
    companion object {
        val None = SubtaskDependencies(enabled = false, refusal = { _, _ -> null }, onDrop = { _, _ -> })
    }
}

/**
 * One drag in progress over the subtask card.
 *
 * Positions are in root coordinates: the finger is reported relative to the row
 * it started on, and the only frame every row and the tooltip share is the root.
 * The row bounds are plain bookkeeping rather than state — they change on every
 * scroll frame, and nothing is drawn from them directly.
 */
@Stable
class SubtaskDragState {
    var draggedLocalId by mutableStateOf<Long?>(null)
        private set
    var pointer by mutableStateOf(Offset.Zero)
        private set
    var hoverLocalId by mutableStateOf<Long?>(null)
        private set

    private val bounds = HashMap<Long, Rect>()

    fun place(
        localId: Long,
        rect: Rect,
    ) {
        bounds[localId] = rect
    }

    fun forget(localId: Long) {
        bounds.remove(localId)
    }

    fun start(
        localId: Long,
        offsetInRow: Offset,
    ) {
        val row = bounds[localId] ?: return
        draggedLocalId = localId
        pointer = row.topLeft + offsetInRow
        hoverLocalId = null
    }

    /** Moves the finger; answers true when it crossed onto a different row. */
    fun move(by: Offset): Boolean {
        val dragged = draggedLocalId ?: return false
        pointer += by
        val over = bounds.entries.firstOrNull { (id, rect) -> id != dragged && rect.contains(pointer) }?.key
        if (over == hoverLocalId) return false
        hoverLocalId = over
        return over != null
    }

    /** Ends the drag; answers the (dragged, target) pair the finger was lifted over, if any. */
    fun finish(): Pair<Long, Long>? {
        val dragged = draggedLocalId
        val target = hoverLocalId
        reset()
        return if (dragged != null && target != null) dragged to target else null
    }

    fun reset() {
        draggedLocalId = null
        hoverLocalId = null
    }
}

/**
 * Makes a subtask row something that can be picked up and dropped onto another.
 *
 * A long press picks it up — the same gesture a list uses to start a selection,
 * and on this screen the only thing a long press does — and the drag that
 * follows belongs to it rather than to the page's scroll. Only an open row can
 * be picked up: a finished task has nothing left to wait for.
 */
@Composable
fun Modifier.subtaskDragSource(
    row: TaskListRow,
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
): Modifier {
    val localId = row.task.localId
    val haptics = LocalHapticFeedback.current
    val current by rememberUpdatedState(dependencies)
    val placed =
        onGloballyPositioned { drag.place(localId, Rect(it.positionInRoot(), it.size.toSize())) }
    if (!dependencies.enabled || row.task.status != TaskStatus.OPEN) return placed
    return placed.pointerInput(localId) {
        detectDragGesturesAfterLongPress(
            onDragStart = { offset ->
                haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                drag.start(localId, offset)
            },
            onDrag = { change, amount ->
                change.consume()
                if (drag.move(amount)) haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
            },
            onDragEnd = {
                val (dragged, target) = drag.finish() ?: return@detectDragGesturesAfterLongPress
                if (current.refusal(dragged, target) == null) current.onDrop(dragged, target)
            },
            onDragCancel = drag::reset,
        )
    }
}

/**
 * The marks a row wears during a drag: faded while it is the one being carried,
 * outlined with the padlock the drop would add while it is under the finger —
 * or, when the drop would be refused, outlined in the error colour with a bar.
 */
@Composable
fun BoxScope.SubtaskDragMarks(
    row: TaskListRow,
    drag: SubtaskDragState,
    refusal: DependencyDropRefusal?,
) {
    if (drag.hoverLocalId != row.task.localId) return
    val color = if (refusal == null) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error
    val onColor = if (refusal == null) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onError
    Box(
        modifier =
            Modifier
                .matchParentSize()
                .border(width = 2.dp, color = color, shape = RoundedCornerShape(8.dp)),
    )
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
            imageVector = if (refusal == null) Icons.Filled.Lock else Icons.Filled.Block,
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

/**
 * What the drop would do, said next to the finger while it is still down.
 *
 * It floats above the finger rather than below it, where the hand would cover
 * it, and over a row it names that row: "will depend on …", or why not. Away
 * from any row it shows what is being carried, so the drag is visibly alive.
 */
@Composable
fun SubtaskDragTooltip(
    drag: SubtaskDragState,
    rows: List<TaskListRow>,
    dependencies: SubtaskDependencies,
) {
    val dragged = drag.draggedLocalId ?: return
    val hover = drag.hoverLocalId
    val target = hover?.let { id -> rows.firstOrNull { it.task.localId == id } }
    val carried = rows.firstOrNull { it.task.localId == dragged } ?: return
    val refusal = target?.let { dependencies.refusal(dragged, it.task.localId) }
    val lift = with(LocalDensity.current) { TOOLTIP_LIFT_DP.dp.roundToPx() }
    val edge = with(LocalDensity.current) { TOOLTIP_EDGE_DP.dp.roundToPx() }

    Popup(popupPositionProvider = AboveFinger(drag.pointer, lift, edge)) {
        Surface(
            shape = RoundedCornerShape(10.dp),
            color = MaterialTheme.colorScheme.inverseSurface,
            contentColor = MaterialTheme.colorScheme.inverseOnSurface,
            shadowElevation = 6.dp,
        ) {
            Row(
                modifier = Modifier.widthIn(max = 320.dp).padding(horizontal = 12.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (target != null) {
                    Icon(
                        imageVector = if (refusal == null) Icons.Filled.Lock else Icons.Filled.Block,
                        contentDescription = null,
                        tint =
                            if (refusal == null) {
                                MaterialTheme.colorScheme.inversePrimary
                            } else {
                                MaterialTheme.colorScheme.error
                            },
                        modifier = Modifier.size(16.dp),
                    )
                }
                Text(
                    text = if (target == null) carried.task.title else tooltipText(refusal, target.task.title),
                    style = MaterialTheme.typography.labelLarge,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

@Composable
private fun tooltipText(
    refusal: DependencyDropRefusal?,
    title: String,
): String =
    when (refusal) {
        null -> stringResource(R.string.page_task_dependencyDrag_willDepend, title)
        DependencyDropRefusal.ANCESTOR -> stringResource(R.string.page_task_dependencyDrag_refusedAncestor)
        DependencyDropRefusal.DESCENDANT -> stringResource(R.string.page_task_dependencyDrag_refusedDescendant)
        DependencyDropRefusal.COMPLETED -> stringResource(R.string.page_task_dependencyDrag_refusedCompleted, title)
        DependencyDropRefusal.EXISTS -> stringResource(R.string.page_task_dependencyDrag_refusedExists, title)
        DependencyDropRefusal.CYCLE -> stringResource(R.string.page_task_dependencyDrag_refusedCycle, title)
    }

/** Centres the tooltip above a point in root coordinates, kept inside the window. */
private class AboveFinger(
    private val pointer: Offset,
    private val lift: Int,
    private val edge: Int,
) : PopupPositionProvider {
    override fun calculatePosition(
        anchorBounds: IntRect,
        windowSize: IntSize,
        layoutDirection: LayoutDirection,
        popupContentSize: IntSize,
    ): IntOffset {
        val x =
            (pointer.x.toInt() - popupContentSize.width / 2)
                .coerceIn(edge, (windowSize.width - popupContentSize.width - edge).coerceAtLeast(edge))
        val y = (pointer.y.toInt() - lift - popupContentSize.height).coerceAtLeast(edge)
        return IntOffset(x, y)
    }
}

private const val DRAGGED_ALPHA = 0.4f
private const val TOOLTIP_LIFT_DP = 24
private const val TOOLTIP_EDGE_DP = 8
