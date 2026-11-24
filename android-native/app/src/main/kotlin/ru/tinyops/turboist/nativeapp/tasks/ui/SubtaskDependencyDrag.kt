package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.PointerInputScope
import androidx.compose.ui.input.pointer.changedToUpIgnoreConsumed
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChangeIgnoreConsumed
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.layout
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.platform.InspectorInfo
import androidx.compose.ui.platform.LocalHapticFeedback
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.DependencyDropRefusal
import ru.tinyops.turboist.core.model.view.NestDropRefusal
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import kotlin.math.roundToInt

/**
 * What the rows need to take part in the drop gestures — on the subtask card of
 * a task, and across the whole board of a project: dropping one onto another to
 * make it wait for it, to nest it as that row's subtask, or lifting it out to the
 * top level (see [SubtaskDropAction]).
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
    /** Lifts the dropped row out to the top level; `null` where the screen offers no detach at all. */
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

/** What a release over the drag panel asks for. */
enum class SubtaskDropAction {
    /** Become a subtask of the row it was carried onto. */
    NEST,

    /** Wait for the row it was carried onto. */
    DEPENDENCY,

    /** Leave its parent for the top level of the same project and section. */
    DETACH,
}

/** What a drag was let go over: an action, and the row it was hovered on — if the action needs one. */
data class SubtaskDrop(
    val draggedLocalId: Long,
    val action: SubtaskDropAction,
    val targetLocalId: Long?,
)

/**
 * One drag in progress over the subtask card or a project board.
 *
 * Positions are in root coordinates: the finger is reported relative to the row
 * it started on, and the only frame every row and the panel share is the root.
 * The row bounds are plain bookkeeping rather than state — they change on every
 * scroll frame, and nothing is drawn from them directly.
 *
 * Two kinds of target are tracked. The rows say which task the drop is about;
 * the panel floats over the list with one icon per action, and says what is being
 * asked of it. The panel is hit first because it is drawn on top, and a finger
 * over the panel keeps the last hovered row as its target — the gesture is "point
 * at a task, then release on an action", and moving onto the panel must not
 * forget which task was pointed at.
 */
@Stable
class SubtaskDragState {
    var draggedLocalId by mutableStateOf<Long?>(null)
        private set
    var hoverLocalId by mutableStateOf<Long?>(null)
        private set

    /** The action the finger is over on the panel, or `null` while it is over the list. */
    var hoveredAction by mutableStateOf<SubtaskDropAction?>(null)
        private set

    /**
     * Where the row the drag is pointed at sits; the panel opens beside it. While
     * nothing is pointed at it is the carried row, so a drag that has not reached
     * anything yet still has somewhere to put its panel.
     */
    var anchor by mutableStateOf<Rect?>(null)
        private set

    // The finger's own position is bookkeeping, never drawn: keeping it out of
    // state means a move only recomposes the screen when the target or the action
    // under it really changed, rather than on every frame of the gesture.
    private var pointer: Offset = Offset.Zero
    private val rows = HashMap<Long, RowBounds>()
    private var carried: Rect? = null
    private var panel: Rect? = null
    private val actionBounds = HashMap<SubtaskDropAction, Rect>()

    /** Where a row is, as a fixed rectangle in root coordinates. */
    fun place(
        localId: Long,
        rect: Rect,
    ) {
        rows[localId] = RowBounds(rect, null)
    }

    /**
     * Where a row is laid out, as its layout handle. The row is measured only
     * while something is being carried: it moves on every scroll frame, and
     * keeping a rectangle current would charge every scroll for the rare one
     * that carries a row.
     */
    internal fun placeRow(
        localId: Long,
        coordinates: LayoutCoordinates,
    ) {
        rows[localId] = RowBounds(null, coordinates)
    }

    /** Drops the handle a row registered, unless the row has registered a newer one since. */
    internal fun forgetRow(
        localId: Long,
        coordinates: LayoutCoordinates?,
    ) {
        if (coordinates != null && rows[localId]?.coordinates === coordinates) rows.remove(localId)
    }

    /**
     * A row has left the screen. If it was being carried the drag is over — its
     * gesture went with it and nothing else would ever end it — and if it was
     * hovered it is no longer something to drop onto.
     */
    fun forget(localId: Long) {
        rows.remove(localId)
        abandon(localId)
        if (hoverLocalId == localId) hoverLocalId = null
    }

    /** Where the panel is on screen, or `null` once it is gone. */
    fun placePanel(rect: Rect?) {
        panel = rect
        if (rect == null) hoveredAction = null
    }

    /** Where one action of the panel is on screen. */
    fun placeAction(
        action: SubtaskDropAction,
        rect: Rect?,
    ) {
        if (rect == null) actionBounds.remove(action) else actionBounds[action] = rect
    }

    /** Ends the drag without a drop, if [localId] is the row being carried. */
    fun abandon(localId: Long) {
        if (draggedLocalId == localId) reset()
    }

    fun start(
        localId: Long,
        offsetInRow: Offset,
    ) {
        val row = rows[localId]?.current() ?: return
        draggedLocalId = localId
        carried = row
        anchor = row
        pointer = row.topLeft + offsetInRow
        hoverLocalId = null
        hoveredAction = null
    }

    /** Moves the finger; answers true when what the panel says would change. */
    fun move(by: Offset): Boolean {
        val dragged = draggedLocalId ?: return false
        pointer += by
        return if (panel?.contains(pointer) == true) hoverAction() else hoverRow(dragged)
    }

    /** The finger is over the panel: only the action under it matters. */
    private fun hoverAction(): Boolean {
        val next = actionBounds.entries.firstOrNull { it.value.contains(pointer) }?.key
        val changed = next != hoveredAction
        hoveredAction = next
        return changed
    }

    /**
     * The finger is over the list. The target is the row under it, and the panel
     * moves beside that row — never on top of it, so the row under the finger
     * stays the target while the finger travels up to an action.
     */
    private fun hoverRow(dragged: Long): Boolean {
        val leftAction = hoveredAction != null
        hoveredAction = null
        val hit =
            rows.entries.firstNotNullOfOrNull { (id, row) ->
                if (id == dragged) null else row.current()?.takeIf { it.contains(pointer) }?.let { id to it }
            }
        if (hit?.first == hoverLocalId) return leftAction
        hoverLocalId = hit?.first
        anchor = hit?.second ?: carried
        return true
    }

    /** Ends the drag; answers what the finger was lifted over, if it was an action. */
    fun finish(): SubtaskDrop? {
        val dragged = draggedLocalId
        val action = hoveredAction
        val target = hoverLocalId
        reset()
        if (dragged == null || action == null) return null
        if (action != SubtaskDropAction.DETACH && target == null) return null
        return SubtaskDrop(dragged, action, if (action == SubtaskDropAction.DETACH) null else target)
    }

    fun reset() {
        draggedLocalId = null
        hoverLocalId = null
        hoveredAction = null
        carried = null
        anchor = null
    }
}

/** Where one row is: fixed, or read from its layout handle at the moment it is asked. */
private class RowBounds(
    private val fixed: Rect?,
    val coordinates: LayoutCoordinates?,
) {
    fun current(): Rect? = fixed ?: coordinates?.rootBounds()
}

private class SubtaskRowNode(
    var localId: Long,
    var drag: SubtaskDragState,
) : DragPlacementNode() {
    override fun register(coordinates: LayoutCoordinates) = drag.placeRow(localId, coordinates)

    override fun unregister(coordinates: LayoutCoordinates?) = drag.forgetRow(localId, coordinates)

    fun update(
        localId: Long,
        drag: SubtaskDragState,
    ) {
        if (localId == this.localId && drag === this.drag) return
        moved {
            this.localId = localId
            this.drag = drag
        }
    }
}

private data class SubtaskRowElement(
    val localId: Long,
    val drag: SubtaskDragState,
) : ModifierNodeElement<SubtaskRowNode>() {
    override fun create() = SubtaskRowNode(localId, drag)

    override fun update(node: SubtaskRowNode) = node.update(localId, drag)

    override fun InspectorInfo.inspectableProperties() {
        name = "subtaskDragRow"
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
    val placed = then(SubtaskRowElement(localId, drag))
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
            // nobody ends leaves its panel hanging over the page.
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
internal suspend fun PointerInputScope.detectLongPressDrag(
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
    when (drop.action) {
        SubtaskDropAction.DETACH -> dependencies.onDetachDrop?.invoke(drop.draggedLocalId)
        SubtaskDropAction.DEPENDENCY -> {
            val target = drop.targetLocalId ?: return
            if (dependencies.dependencyRefusal(drop.draggedLocalId, target) == null) {
                dependencies.onDependencyDrop(drop.draggedLocalId, target)
            }
        }
        SubtaskDropAction.NEST -> {
            val target = drop.targetLocalId ?: return
            if (!dependencies.nestNoop(drop.draggedLocalId, target) &&
                dependencies.nestRefusal(drop.draggedLocalId, target) == null
            ) {
                dependencies.onNestDrop(drop.draggedLocalId, target)
            }
        }
    }
}

/**
 * Places the tooltip centred on one side of a point in root coordinates [lift]
 * away from it, kept inside the area it is laid out in. The side is chosen so the
 * line never lands on a panel that opens over the finger; the point is read while
 * placing, so a moving finger re-places the tooltip without recomposing it.
 */
internal fun Modifier.fingerSide(
    pointer: () -> Offset,
    lift: Int,
    edge: Int,
    above: Boolean,
): Modifier =
    layout { measurable, constraints ->
        val placeable = measurable.measure(constraints.copy(minWidth = 0, minHeight = 0))
        val width = constraints.maxWidth
        val height = constraints.maxHeight
        layout(width, height) {
            val origin = coordinates?.positionInRoot() ?: Offset.Zero
            val finger = pointer() - origin
            val x =
                (finger.x.roundToInt() - placeable.width / 2)
                    .coerceIn(edge, (width - placeable.width - edge).coerceAtLeast(edge))
            val y =
                if (above) {
                    (finger.y.roundToInt() - lift - placeable.height).coerceAtLeast(edge)
                } else {
                    (finger.y.roundToInt() + lift)
                        .coerceAtMost((height - placeable.height - edge).coerceAtLeast(edge))
                }
            placeable.place(x, y)
        }
    }
