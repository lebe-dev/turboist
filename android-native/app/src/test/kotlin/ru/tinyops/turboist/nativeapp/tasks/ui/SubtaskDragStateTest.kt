package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The bookkeeping behind dragging one task onto another.
 *
 * Two things are checked hardest. The first is that the panel is how a drop is
 * committed: the row under the finger is only ever the target, and an action has
 * to be picked. The second is that a drag always ends — a row that leaves the
 * screen while it is being carried (a sync rewrote the list, the row finished
 * elsewhere) must not leave a panel hanging over the page with nothing able to
 * take it down.
 */
class SubtaskDragStateTest {
    private fun board(): SubtaskDragState =
        SubtaskDragState().apply {
            place(1, Rect(0f, 0f, 400f, 50f))
            place(2, Rect(0f, 50f, 400f, 100f))
            placePanel(Rect(0f, 150f, 400f, 250f))
            placeAction(SubtaskDropAction.NEST, Rect(0f, 180f, 130f, 240f))
            placeAction(SubtaskDropAction.DEPENDENCY, Rect(133f, 180f, 263f, 240f))
            placeAction(SubtaskDropAction.DETACH, Rect(266f, 180f, 396f, 240f))
        }

    /** Starts carrying row 1 with the finger inside it. */
    private fun SubtaskDragState.pickUp() = start(1, Offset(10f, 25f))

    @Test
    fun `pointing at a row and releasing on an action names both`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))
        assertEquals(2L, drag.hoverLocalId, "the row under the finger is the target")

        // Up onto the panel, over its first action.
        drag.move(Offset(0f, 130f))

        assertEquals(SubtaskDropAction.NEST, drag.hoveredAction)
        assertEquals(SubtaskDrop(1, SubtaskDropAction.NEST, 2), drag.finish())
        assertNull(drag.draggedLocalId)
        assertNull(drag.hoveredAction)
    }

    @Test
    fun `reaching an action keeps the last pointed-at row as the target`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))
        drag.move(Offset(0f, 130f))
        drag.move(Offset(140f, 0f))

        assertEquals(SubtaskDropAction.DEPENDENCY, drag.hoveredAction)
        assertEquals(2L, drag.hoverLocalId, "moving onto the panel must not forget the target")
        assertEquals(SubtaskDrop(1, SubtaskDropAction.DEPENDENCY, 2), drag.finish())
    }

    @Test
    fun `lifting the row out needs no second row`() {
        val drag = board()
        drag.start(2, Offset(10f, 25f))
        drag.move(Offset(0f, 130f))
        drag.move(Offset(270f, 0f))

        assertEquals(SubtaskDropAction.DETACH, drag.hoveredAction)
        assertEquals(SubtaskDrop(2, SubtaskDropAction.DETACH, null), drag.finish())
    }

    @Test
    fun `releasing over a row without picking an action drops nothing`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))

        assertEquals(1L, drag.draggedLocalId, "the row stays carried while no action is picked")
        assertNull(drag.finish())
    }

    @Test
    fun `leaving the panel for the list keeps the target and drops the action`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))
        drag.move(Offset(0f, 130f))
        assertTrue(drag.move(Offset(0f, -130f)))

        assertNull(drag.hoveredAction)
        assertEquals(2L, drag.hoverLocalId)
    }

    @Test
    fun `a panel that leaves the screen takes its chosen action with it`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))
        drag.move(Offset(0f, 130f))
        assertEquals(SubtaskDropAction.NEST, drag.hoveredAction)

        drag.placePanel(null)

        assertNull(drag.hoveredAction)
        assertNull(drag.finish())
    }

    @Test
    fun `a carried row that leaves the screen ends the drag`() {
        val drag = board()
        drag.pickUp()

        drag.forget(1)

        assertNull(drag.draggedLocalId)
        assertNull(drag.finish())
    }

    @Test
    fun `a hovered row that leaves the screen is no longer a target`() {
        val drag = board()
        drag.pickUp()
        drag.move(Offset(0f, 50f))

        drag.forget(2)

        assertEquals(1L, drag.draggedLocalId)
        assertNull(drag.hoverLocalId)
        assertNull(drag.finish())
    }

    @Test
    fun `abandoning a row that is not the one carried leaves the drag alone`() {
        val drag = board()
        drag.pickUp()

        drag.abandon(2)

        assertEquals(1L, drag.draggedLocalId)
        assertNull(drag.hoveredAction)
    }
}
