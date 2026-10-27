package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.SubdirectoryArrowRight
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.PointerInputScope
import androidx.compose.ui.input.pointer.changedToUpIgnoreConsumed
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChangeIgnoreConsumed
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
import ru.tinyops.turboist.core.model.view.NestDropRefusal
import ru.tinyops.turboist.core.model.view.SubtaskDropMode
import ru.tinyops.turboist.core.model.view.subtaskDropMode
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow

/**
 * What the rows need to take part in the two related drop gestures — on the
 * subtask card of a task, and across the whole board of a project:
 * dropping one onto the edge of another to make it wait for it, or onto its
 * middle to nest it as that row's subtask (see [SubtaskDropMode]).
 *
 * The refusal lambdas are asked on every hover, so they have to be a cheap read
 * of the replica — they are: the screen state already holds the tree and the
 * edges. [nestNoop] is not a refusal — it says the nest would change nothing
 * (the target is already the dragged row's parent) — so it is checked apart
 * from [nestRefusal] rather than folded into it.
 */
class SubtaskDependencies(
    val enabled: Boolean,
    val dependencyRefusal: (draggedLocalId: Long, targetLocalId: Long) -> DependencyDropRefusal?,
    val nestRefusal: (draggedLocalId: Long, targetLocalId: Long) -> NestDropRefusal?,
    val nestNoop: (draggedLocalId: Long, targetLocalId: Long) -> Boolean,
    val onDependencyDrop: (draggedLocalId: Long, targetLocalId: Long) -> Unit,
    val onNestDrop: (draggedLocalId: Long, targetLocalId: Long) -> Unit,
    /** Lifts the dropped row out to the top level; `null` where the screen offers no detach zone. */
    val onDetachDrop: ((draggedLocalId: Long) -> Unit)? = null,
) {
    companion object {
        val None =
            SubtaskDependencies(
                enabled = false,
                dependencyRefusal = { _, _ -> null },
                nestRefusal = { _, _ -> null },
                nestNoop = { _, _ -> false },
                onDependencyDrop = { _, _ -> },
                onNestDrop = { _, _ -> },
            )
    }
}

/** What a drag was let go over. */
sealed interface SubtaskDrop {
    /** Another row, at an edge or in its middle (see [SubtaskDropMode]). */
    data class OntoRow(
        val draggedLocalId: Long,
        val targetLocalId: Long,
        val mode: SubtaskDropMode,
    ) : SubtaskDrop

    /** The detach zone: the row leaves its parent for the top level. */
    data class Detach(val draggedLocalId: Long) : SubtaskDrop
}

/**
 * One drag in progress over the subtask card.
 *
 * Positions are in root coordinates: the finger is reported relative to the row
 * it started on, and the only frame every row and the tooltip share is the root.
 * The row bounds are plain bookkeeping rather than state — they change on every
 * scroll frame, and nothing is drawn from them directly. [mode] is recomputed on
 * every move from where inside the hovered row's own height the finger sits, so
 * crossing from the middle band into an edge switches modes mid-drag.
 */
@Stable
class SubtaskDragState {
    var draggedLocalId by mutableStateOf<Long?>(null)
        private set
    var pointer by mutableStateOf(Offset.Zero)
        private set
    var hoverLocalId by mutableStateOf<Long?>(null)
        private set
    var mode by mutableStateOf<SubtaskDropMode?>(null)
        private set
    var overDetachZone by mutableStateOf(false)
        private set

    private val bounds = HashMap<Long, Rect>()
    private var detachZone: Rect? = null

    fun place(
        localId: Long,
        rect: Rect,
    ) {
        bounds[localId] = rect
    }

    /** Where the detach zone is on screen, or `null` once it is gone. */
    fun placeDetachZone(rect: Rect?) {
        detachZone = rect
        if (rect == null) overDetachZone = false
    }

    /**
     * A row has left the screen. If it was being carried the drag is over — its
     * gesture went with it and nothing else would ever end it — and if it was
     * hovered it is no longer something to drop onto.
     */
    fun forget(localId: Long) {
        bounds.remove(localId)
        abandon(localId)
        if (hoverLocalId == localId) {
            hoverLocalId = null
            mode = null
        }
    }

    /** Ends the drag without a drop, if [localId] is the row being carried. */
    fun abandon(localId: Long) {
        if (draggedLocalId == localId) reset()
    }

    fun start(
        localId: Long,
        offsetInRow: Offset,
    ) {
        val row = bounds[localId] ?: return
        draggedLocalId = localId
        pointer = row.topLeft + offsetInRow
        hoverLocalId = null
        mode = null
        overDetachZone = false
    }

    /** Moves the finger; answers true when the hovered row, the mode or the detach zone changed. */
    fun move(by: Offset): Boolean {
        val dragged = draggedLocalId ?: return false
        pointer += by
        val inZone = detachZone?.contains(pointer) == true
        if (inZone) {
            val changed = !overDetachZone || hoverLocalId != null
            overDetachZone = true
            hoverLocalId = null
            mode = null
            return changed
        }
        val leftZone = overDetachZone
        overDetachZone = false
        val hit = bounds.entries.firstOrNull { (id, rect) -> id != dragged && rect.contains(pointer) }
        val newMode = hit?.let { subtaskDropMode(((pointer.y - it.value.top) / it.value.height)) }
        if (!leftZone && hit?.key == hoverLocalId && newMode == mode) return false
        hoverLocalId = hit?.key
        mode = newMode
        return true
    }

    /** Ends the drag; answers what the finger was lifted over, if it was anything to drop onto. */
    fun finish(): SubtaskDrop? {
        val dragged = draggedLocalId
        val target = hoverLocalId
        val droppedMode = mode
        val detach = overDetachZone
        reset()
        return when {
            dragged == null -> null
            detach -> SubtaskDrop.Detach(dragged)
            target != null && droppedMode != null -> SubtaskDrop.OntoRow(dragged, target, droppedMode)
            else -> null
        }
    }

    fun reset() {
        draggedLocalId = null
        hoverLocalId = null
        mode = null
        overDetachZone = false
    }
}

/**
 * Makes a subtask row something that can be picked up and dropped onto another.
 *
 * A long press picks it up — the same gesture a list uses to start a selection,
 * and on this screen the only thing a long press does — and the drag that
 * follows belongs to it rather than to the page's scroll. Only an open row can
 * be picked up: a finished task has nothing left to wait for and nothing useful
 * to nest under either.
 *
 * [onPressWithoutDrag] is what a long press means when the finger lifts without
 * having moved: on a screen where a long press already had a job of its own,
 * picking the row up must not take that job away.
 */
@Composable
fun Modifier.subtaskDragSource(
    row: TaskListRow,
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
    onPressWithoutDrag: (() -> Unit)? = null,
): Modifier {
    val localId = row.task.localId
    val haptics = LocalHapticFeedback.current
    val current by rememberUpdatedState(dependencies)
    val pressed by rememberUpdatedState(onPressWithoutDrag)
    val placed =
        onGloballyPositioned { drag.place(localId, Rect(it.positionInRoot(), it.size.toSize())) }
    if (!dependencies.enabled || row.task.status != TaskStatus.OPEN) return placed
    return placed.pointerInput(localId) {
        var travelled = 0f
        try {
            detectLongPressDrag(
                onStart = { offset ->
                    haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                    travelled = 0f
                    drag.start(localId, offset)
                },
                onMove = { amount ->
                    travelled += amount.getDistance()
                    if (drag.move(amount)) haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                },
                onEnd = {
                    val drop = drag.finish()
                    when {
                        drop != null -> deliver(drop, current)
                        travelled < viewConfiguration.touchSlop -> pressed?.invoke()
                    }
                },
            )
        } finally {
            // The gesture is torn down without a cancel when the row stops being
            // draggable mid-drag — it finished, or the parent did — and a drag
            // nobody ends leaves its tooltip hanging over the page.
            drag.abandon(localId)
        }
    }
}

/**
 * A long press that picks something up, and the drag that follows it.
 *
 * Read in the initial pass, before the row inside and the scrolling page around
 * it see the events. Until the press has lasted long enough nothing is consumed,
 * so a tap still opens the row and a swipe still scrolls; once it has, every
 * event is consumed here. The stock detector reads the main pass instead, where
 * the row's own long-click handling consumes the rest of the gesture first and
 * the drag is cancelled the moment the finger moves.
 */
private suspend fun PointerInputScope.detectLongPressDrag(
    onStart: (Offset) -> Unit,
    onMove: (Offset) -> Unit,
    onEnd: () -> Unit,
) {
    awaitEachGesture {
        val down = awaitFirstDown(requireUnconsumed = false, pass = PointerEventPass.Initial)
        // Answers false when the gesture turned out to be something else — a tap,
        // a scroll — and null when the timeout ran out with the finger still put.
        val interrupted =
            withTimeoutOrNull(viewConfiguration.longPressTimeoutMillis) {
                var other = false
                while (!other) {
                    val change =
                        awaitPointerEvent(PointerEventPass.Initial).changes.firstOrNull { it.id == down.id }
                    other = change == null ||
                        change.changedToUpIgnoreConsumed() ||
                        change.isConsumed ||
                        (change.position - down.position).getDistance() > viewConfiguration.touchSlop
                }
                false
            }
        if (interrupted != null) return@awaitEachGesture
        onStart(down.position)
        while (true) {
            val change =
                awaitPointerEvent(PointerEventPass.Initial).changes.firstOrNull { it.id == down.id } ?: break
            val amount = change.positionChangeIgnoreConsumed()
            change.consume()
            if (change.changedToUpIgnoreConsumed()) break
            if (amount != Offset.Zero) onMove(amount)
        }
        onEnd()
    }
}

private fun deliver(
    drop: SubtaskDrop,
    dependencies: SubtaskDependencies,
) {
    when (drop) {
        is SubtaskDrop.Detach -> dependencies.onDetachDrop?.invoke(drop.draggedLocalId)
        is SubtaskDrop.OntoRow -> {
            val (dragged, target, mode) = drop
            when (mode) {
                SubtaskDropMode.DEPENDENCY ->
                    if (dependencies.dependencyRefusal(dragged, target) == null) {
                        dependencies.onDependencyDrop(dragged, target)
                    }
                SubtaskDropMode.NEST ->
                    if (!dependencies.nestNoop(dragged, target) && dependencies.nestRefusal(dragged, target) == null) {
                        dependencies.onNestDrop(dragged, target)
                    }
            }
        }
    }
}

/**
 * Where a carried subtask is dropped to leave its parent for the top level of
 * the same project and section. It exists only while a drag is under way, and
 * the screen floats it over its own foot rather than laying it out among the
 * rows: in the page it could sit below the fold, and appearing there would
 * push the rows out from under the finger that just picked one up.
 */
@Composable
fun SubtaskDetachZone(
    drag: SubtaskDragState,
    modifier: Modifier = Modifier,
) {
    DisposableEffect(drag) { onDispose { drag.placeDetachZone(null) } }
    val active = drag.overDetachZone
    val outline = if (active) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline
    Box(
        modifier =
            modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .onGloballyPositioned { drag.placeDetachZone(Rect(it.positionInRoot(), it.size.toSize())) }
                .background(
                    if (active) MaterialTheme.colorScheme.primaryContainer else Color.Transparent,
                    RoundedCornerShape(8.dp),
                ).border(width = 1.dp, color = outline, shape = RoundedCornerShape(8.dp))
                .padding(horizontal = 12.dp, vertical = 12.dp),
    ) {
        Text(
            text = stringResource(R.string.page_task_detachDrag_hint),
            style = MaterialTheme.typography.bodySmall,
            color =
                if (active) {
                    MaterialTheme.colorScheme.onPrimaryContainer
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
        )
    }
}

/**
 * The marks a row wears during a drag: outlined with the padlock the drop
 * would add when hovered at an edge, or the indent arrow when hovered at its
 * middle — or, either way, outlined in the error colour when the drop would be
 * refused. A nest that would be a no-op (already its parent) shows nothing,
 * the same way hovering the dragged row itself shows nothing.
 */
@Composable
fun BoxScope.SubtaskDragMarks(
    row: TaskListRow,
    drag: SubtaskDragState,
    dependencies: SubtaskDependencies,
) {
    val dragged = drag.draggedLocalId ?: return
    val mode = drag.mode ?: return
    if (drag.hoverLocalId != row.task.localId) return
    if (mode == SubtaskDropMode.NEST && dependencies.nestNoop(dragged, row.task.localId)) return
    val refused = isRefused(mode, dependencies, dragged, row.task.localId)
    val color =
        when {
            refused -> MaterialTheme.colorScheme.error
            mode == SubtaskDropMode.NEST -> NEST_COLOR
            else -> MaterialTheme.colorScheme.primary
        }
    val onColor = if (refused) MaterialTheme.colorScheme.onError else Color.White
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
            imageVector = dropIcon(refused, mode),
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
 * it, and over a row it names that row: "will depend on …" or "will become a
 * subtask of …", or why not. Away from any row it shows what is being carried,
 * so the drag is visibly alive.
 */
@Composable
fun SubtaskDragTooltip(
    drag: SubtaskDragState,
    rows: List<TaskListRow>,
    dependencies: SubtaskDependencies,
) {
    val dragged = drag.draggedLocalId ?: return
    val hover = drag.hoverLocalId
    val mode = drag.mode
    val target = hover?.let { id -> rows.firstOrNull { it.task.localId == id } }
    val carried = rows.firstOrNull { it.task.localId == dragged } ?: return
    // Nothing to show over a row without a settled mode, or over a nest that
    // would be a no-op — both read as "not really hovering a target".
    val showTarget =
        target != null && mode != null &&
            !(mode == SubtaskDropMode.NEST && dependencies.nestNoop(dragged, target.task.localId))
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
                if (showTarget && target != null && mode != null) {
                    val refused = isRefused(mode, dependencies, dragged, target.task.localId)
                    Icon(
                        imageVector = dropIcon(refused, mode),
                        contentDescription = null,
                        tint =
                            if (refused) {
                                MaterialTheme.colorScheme.error
                            } else {
                                MaterialTheme.colorScheme.inversePrimary
                            },
                        modifier = Modifier.size(16.dp),
                    )
                }
                Text(
                    text =
                        if (showTarget && target != null && mode != null) {
                            tooltipText(mode, dependencies, dragged, target.task.localId, target.task.title)
                        } else {
                            carried.task.title
                        },
                    style = MaterialTheme.typography.labelLarge,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

private fun isRefused(
    mode: SubtaskDropMode,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    targetLocalId: Long,
): Boolean =
    when (mode) {
        SubtaskDropMode.DEPENDENCY -> dependencies.dependencyRefusal(draggedLocalId, targetLocalId) != null
        SubtaskDropMode.NEST -> dependencies.nestRefusal(draggedLocalId, targetLocalId) != null
    }

/** The icon a mark or tooltip shows: refused always wins, otherwise per mode. */
private fun dropIcon(
    refused: Boolean,
    mode: SubtaskDropMode,
): ImageVector =
    when {
        refused -> Icons.Filled.Block
        mode == SubtaskDropMode.NEST -> Icons.Filled.SubdirectoryArrowRight
        else -> Icons.Filled.Lock
    }

@Composable
private fun tooltipText(
    mode: SubtaskDropMode,
    dependencies: SubtaskDependencies,
    draggedLocalId: Long,
    targetLocalId: Long,
    title: String,
): String =
    if (mode == SubtaskDropMode.NEST) {
        when (dependencies.nestRefusal(draggedLocalId, targetLocalId)) {
            null -> stringResource(R.string.page_task_nestDrag_willNest, title)
            NestDropRefusal.DESCENDANT -> stringResource(R.string.page_task_nestDrag_refusedDescendant)
            NestDropRefusal.COMPLETED -> stringResource(R.string.page_task_nestDrag_refusedCompleted, title)
        }
    } else {
        when (dependencies.dependencyRefusal(draggedLocalId, targetLocalId)) {
            null -> stringResource(R.string.page_task_dependencyDrag_willDepend, title)
            DependencyDropRefusal.ANCESTOR -> stringResource(R.string.page_task_dependencyDrag_refusedAncestor)
            DependencyDropRefusal.DESCENDANT -> stringResource(R.string.page_task_dependencyDrag_refusedDescendant)
            DependencyDropRefusal.COMPLETED -> stringResource(R.string.page_task_dependencyDrag_refusedCompleted, title)
            DependencyDropRefusal.EXISTS -> stringResource(R.string.page_task_dependencyDrag_refusedExists, title)
            DependencyDropRefusal.CYCLE -> stringResource(R.string.page_task_dependencyDrag_refusedCycle, title)
        }
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

// The same violet accent the web client uses for a nest drop, distinct from the
// primary blue a dependency drop uses — the two gestures must never look alike.
private val NEST_COLOR = Color(0xFF8B5CF6)

private const val DRAGGED_ALPHA = 0.4f
private const val TOOLTIP_LIFT_DP = 24
private const val TOOLTIP_EDGE_DP = 8
