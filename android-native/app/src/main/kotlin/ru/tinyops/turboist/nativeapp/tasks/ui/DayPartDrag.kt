package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.layout
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.toSize
import androidx.compose.ui.window.Popup
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import kotlin.math.roundToInt

/** The phases a row can be carried to, in the order the day reads. */
internal val DROP_DAY_PARTS = listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE)

/**
 * One row being carried between the phases of a day.
 *
 * Positions are in root coordinates, like the subtask drag's: the finger is
 * reported relative to the row it started on, and the root is the only frame
 * the rows, the phase blocks and the drop bar share. The bounds are
 * bookkeeping, not state — they change on every scroll frame and nothing is
 * drawn from them directly.
 *
 * Two kinds of target are tracked. The blocks of the list say which phase
 * whatever sits at that height belongs to; the drop bar floats over the list
 * and names every phase, including the ones with nothing in them yet, which the
 * list does not draw at all. The bar is hit first because it is drawn on top.
 */
@Stable
class DayPartDragState {
    /** The task being carried, or `null` while nothing is. */
    var dragged by mutableStateOf<Task?>(null)
        private set

    /** The phase the carried task was picked up from. */
    var origin by mutableStateOf<DayPart?>(null)
        private set

    var pointer by mutableStateOf(Offset.Zero)
        private set

    /** Where the carried row sat when it was picked up; the island opens next to it. */
    var anchor by mutableStateOf<Rect?>(null)
        private set

    /** The phase under the finger, or `null` over nothing that takes a drop. */
    var hover by mutableStateOf<DayPart?>(null)
        private set

    /** True while the finger is over the drop bar rather than over the list. */
    var overBar by mutableStateOf(false)
        private set

    private val rows = HashMap<Long, Rect>()
    private val blocks = HashMap<String, Pair<DayPart, Rect>>()
    private val bar = HashMap<DayPart, Rect>()

    /** Where a row is on screen, so a drag starting on it knows where the finger is. */
    fun placeRow(
        localId: Long,
        rect: Rect,
    ) {
        rows[localId] = rect
    }

    /**
     * A row has left the screen. If it was being carried the drag is over: its
     * gesture went with it, and nothing else would ever end it.
     */
    fun forgetRow(localId: Long) {
        rows.remove(localId)
        abandon(localId)
    }

    /** Where one item of a phase's block is on screen; [key] names the item. */
    fun placeBlock(
        key: String,
        part: DayPart,
        rect: Rect,
    ) {
        blocks[key] = part to rect
    }

    fun forgetBlock(key: String) {
        blocks.remove(key)
    }

    /** Where one phase of the drop bar is on screen, or `null` once the bar is gone. */
    fun placeBar(
        part: DayPart,
        rect: Rect?,
    ) {
        if (rect == null) bar.remove(part) else bar[part] = rect
    }

    fun start(
        task: Task,
        part: DayPart,
        offsetInRow: Offset,
    ) {
        val row = rows[task.localId] ?: return
        dragged = task
        origin = part
        anchor = row
        pointer = row.topLeft + offsetInRow
        hover = null
        overBar = false
    }

    /** Moves the finger; answers true when the phase under it changed. */
    fun move(by: Offset): Boolean {
        if (dragged == null) return false
        pointer += by
        val onBar = bar.entries.firstOrNull { it.value.contains(pointer) }?.key
        val next = onBar ?: blocks.values.firstOrNull { it.second.contains(pointer) }?.first
        overBar = onBar != null
        if (next == hover) return false
        hover = next
        return true
    }

    /**
     * Ends the drag; answers the task and the phase it was let go over, or
     * `null` when it was let go over nothing — or over the phase it came from,
     * which is a drop that changes nothing.
     */
    fun finish(): Pair<Task, DayPart>? {
        val task = dragged
        val target = hover
        val from = origin
        reset()
        if (task == null || target == null || target == from) return null
        return task to target
    }

    /** Ends the drag without a drop, if [localId] is the row being carried. */
    fun abandon(localId: Long) {
        if (dragged?.localId == localId) reset()
    }

    fun reset() {
        dragged = null
        origin = null
        anchor = null
        hover = null
        overBar = false
    }
}

/** Where the drop island goes: the top edge in root coordinates, and which side of the row it took. */
internal data class IslandPlacement(
    val top: Float,
    val above: Boolean,
)

/**
 * Puts the island of phases right next to the carried row: under it when the
 * list has room there, over it otherwise. When neither side fits it is pinned to
 * the top of the list, which covers the row but keeps every phase reachable.
 */
internal fun islandPlacement(
    anchor: Rect,
    container: Rect,
    islandHeight: Float,
    gap: Float,
): IslandPlacement {
    val below = anchor.bottom + gap
    if (below + islandHeight <= container.bottom) return IslandPlacement(below, above = false)
    val over = anchor.top - gap - islandHeight
    if (over >= container.top) return IslandPlacement(over, above = true)
    return IslandPlacement(container.top + gap, above = false)
}

/**
 * Makes a row of a phase's block something that can be picked up and carried
 * to another phase.
 *
 * A long press picks it up and the drag that follows belongs to it rather than
 * to the list's scroll or the row's swipe. On a list a long press already
 * starts a selection, so a long press the finger lifts from without moving
 * still does exactly that ([onPressWithoutDrag]); only one that moves carries
 * the row. A finished row stays where it is — there is nothing left to plan
 * about it — and so does every row while tasks are being picked.
 */
@Composable
internal fun Modifier.dayPartDragSource(
    task: Task,
    part: DayPart?,
    drag: DayPartDragState?,
    enabled: Boolean,
    onPressWithoutDrag: () -> Unit,
    onDrop: (Task, DayPart) -> Unit,
): Modifier {
    if (drag == null || part == null) return this
    val localId = task.localId
    val haptics = LocalHapticFeedback.current
    val pressed by rememberUpdatedState(onPressWithoutDrag)
    val dropped by rememberUpdatedState(onDrop)
    val current by rememberUpdatedState(task)
    DisposableEffect(drag, localId) { onDispose { drag.forgetRow(localId) } }
    val placed = onGloballyPositioned { drag.placeRow(localId, Rect(it.positionInRoot(), it.size.toSize())) }
    if (!enabled || task.status != TaskStatus.OPEN) return placed
    return placed.pointerInput(localId, part) {
        var travelled = 0f
        try {
            detectLongPressDrag(
                onStart = { offset ->
                    haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                    travelled = 0f
                    drag.start(current, part, offset)
                },
                onMove = { amount ->
                    travelled += amount.getDistance()
                    if (drag.move(amount)) haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                },
                onEnd = {
                    val drop = drag.finish()
                    when {
                        drop != null -> dropped(drop.first, drop.second)
                        travelled < viewConfiguration.touchSlop -> pressed()
                    }
                },
            )
        } finally {
            // Torn down without a cancel when the row stops being draggable
            // mid-drag — it was finished, or a selection started — and a drag
            // nobody ends would leave the bar hanging over the list.
            drag.abandon(localId)
        }
    }
}

/** Registers one item of a phase's block as somewhere a carried row can be dropped. */
@Composable
internal fun Modifier.dayPartDropTarget(
    key: String,
    part: DayPart?,
    drag: DayPartDragState?,
): Modifier {
    if (drag == null || part == null) return this
    DisposableEffect(drag, key) { onDispose { drag.forgetBlock(key) } }
    return onGloballyPositioned { drag.placeBlock(key, part, Rect(it.positionInRoot(), it.size.toSize())) }
}

/** Fades the row that is being carried. */
internal fun Modifier.dayPartDragFade(
    task: Task,
    drag: DayPartDragState?,
): Modifier = if (drag?.dragged?.localId == task.localId) alpha(DRAGGED_ALPHA) else this

/** True when [part] is where the carried row would land, and landing there would change something. */
internal fun DayPartDragState?.wouldLandIn(part: DayPart?): Boolean =
    this != null && part != null && dragged != null && hover == part && origin != part

/**
 * Every phase of the day, opened as an island beside the row being carried.
 *
 * The list leaves out a phase nothing falls into, so without the island an empty
 * evening could not be dropped onto at all; and a long day pushes most of its
 * phases off screen, so the island travels with the row instead of waiting at
 * the top of the list. It floats over the list rather than being laid out in it,
 * so appearing never pushes the rows out from under the finger.
 */
@Composable
internal fun DayPartDropBar(
    drag: DayPartDragState,
    container: Rect,
    modifier: Modifier = Modifier,
) {
    DisposableEffect(drag) { onDispose { DROP_DAY_PARTS.forEach { drag.placeBar(it, null) } } }
    val anchor = drag.anchor ?: return
    val gap = with(LocalDensity.current) { ISLAND_GAP.roundToPx() }.toFloat()
    Surface(
        shape = RoundedCornerShape(16.dp),
        tonalElevation = 3.dp,
        shadowElevation = 6.dp,
        modifier =
            modifier.fillMaxWidth().layout { measurable, constraints ->
                val placeable = measurable.measure(constraints.copy(minHeight = 0))
                val placed = islandPlacement(anchor, container, placeable.height.toFloat(), gap)
                layout(constraints.maxWidth, constraints.maxHeight) {
                    placeable.place(0, (placed.top - container.top).roundToInt())
                }
            },
    ) {
        Column(modifier = Modifier.padding(horizontal = 8.dp, vertical = 8.dp)) {
            Text(
                text = stringResource(R.string.native_dayPartDrag_hint),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 8.dp, bottom = 6.dp),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                for (part in DROP_DAY_PARTS) {
                    DropBarPhase(part, drag, Modifier.weight(1f))
                }
            }
        }
    }
}

/** True when the island would open above the carried row, where the tooltip would cover it. */
@Composable
internal fun DayPartDragState.islandIsAbove(container: Rect): Boolean {
    val anchor = anchor ?: return false
    val density = LocalDensity.current
    val height = with(density) { ISLAND_HEIGHT_ESTIMATE.toPx() }
    val gap = with(density) { ISLAND_GAP.toPx() }
    return islandPlacement(anchor, container, height, gap).above
}

@Composable
private fun DropBarPhase(
    part: DayPart,
    drag: DayPartDragState,
    modifier: Modifier = Modifier,
) {
    val active = drag.wouldLandIn(part)
    // The phase the row came from takes no drop, and says so by stepping back.
    val home = drag.origin == part
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier =
            modifier
                .onGloballyPositioned { drag.placeBar(part, Rect(it.positionInRoot(), it.size.toSize())) }
                .alpha(if (home) HOME_ALPHA else 1f)
                .clip(RoundedCornerShape(12.dp))
                .background(
                    if (active) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surface,
                ).padding(vertical = 8.dp, horizontal = 4.dp),
    ) {
        Icon(
            imageVector = dayPartIcon(part),
            contentDescription = null,
            tint = if (active) dayPartTint(part) else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = stringResource(dayPartLabel(part)),
            style = MaterialTheme.typography.labelSmall,
            color =
                if (active) {
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

/**
 * What the drop would do, said above the finger while it is still down: the
 * phase the row would move to, or — away from any — what is being carried, so
 * the drag is visibly alive. Over the island it says nothing, and neither does it
 * while the island sits above the row: the island lights the phase itself, and a
 * tooltip lifted above the finger would cover it.
 */
@Composable
internal fun DayPartDragTooltip(
    drag: DayPartDragState,
    islandAbove: Boolean,
) {
    val task = drag.dragged ?: return
    if (drag.overBar || islandAbove) return
    val target = drag.hover?.takeIf { drag.wouldLandIn(it) }
    val lift = with(LocalDensity.current) { TOOLTIP_LIFT.roundToPx() }
    val edge = with(LocalDensity.current) { TOOLTIP_EDGE.roundToPx() }
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
                        imageVector = dayPartIcon(target),
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.inversePrimary,
                        modifier = Modifier.size(16.dp),
                    )
                }
                Text(
                    text =
                        if (target != null) {
                            stringResource(R.string.native_dayPartDrag_willMove, stringResource(dayPartLabel(target)))
                        } else {
                            task.title
                        },
                    style = MaterialTheme.typography.labelLarge,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

private val ISLAND_GAP = 8.dp
private val ISLAND_HEIGHT_ESTIMATE = 104.dp
private const val DRAGGED_ALPHA = 0.4f
private const val HOME_ALPHA = 0.45f
private val TOOLTIP_LIFT = 24.dp
private val TOOLTIP_EDGE = 8.dp
