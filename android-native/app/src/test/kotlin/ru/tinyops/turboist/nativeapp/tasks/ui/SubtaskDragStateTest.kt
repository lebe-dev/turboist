package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import ru.tinyops.turboist.core.model.view.SubtaskDropMode
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The bookkeeping behind dragging one subtask onto another.
 *
 * The rule checked hardest is that a drag always ends: a row that leaves the
 * screen while it is being carried — a sync rewrote the list, the row finished
 * elsewhere — must not leave a tooltip hanging over the page with nothing able
 * to take it down.
 */
class SubtaskDragStateTest {
    private fun board(): SubtaskDragState =
        SubtaskDragState().apply {
            place(1, Rect(0f, 0f, 400f, 50f))
            place(2, Rect(0f, 50f, 400f, 100f))
            placeDetachZone(Rect(0f, 200f, 400f, 250f))
        }

    @Test
    fun `lifting the finger over another row's middle drops onto that row`() {
        val drag = board()
        drag.start(1, Offset(10f, 25f))
        drag.move(Offset(0f, 50f))

        assertEquals(SubtaskDrop.OntoRow(1, 2, SubtaskDropMode.NEST), drag.finish())
        assertNull(drag.draggedLocalId)
    }

    @Test
    fun `lifting the finger over the detach zone lifts the row out`() {
        val drag = board()
        drag.start(2, Offset(10f, 25f))

        assertTrue(drag.move(Offset(0f, 150f)), "entering the zone is a change worth a tick")
        assertTrue(drag.overDetachZone)
        assertNull(drag.hoverLocalId)
        assertEquals(SubtaskDrop.Detach(2), drag.finish())
        assertFalse(drag.overDetachZone)
    }

    @Test
    fun `leaving the detach zone for empty space drops nothing`() {
        val drag = board()
        drag.start(2, Offset(10f, 25f))
        drag.move(Offset(0f, 150f))
        drag.move(Offset(0f, 100f))

        assertFalse(drag.overDetachZone)
        assertNull(drag.finish())
    }

    @Test
    fun `a carried row that leaves the screen ends the drag`() {
        val drag = board()
        drag.start(1, Offset(10f, 25f))

        drag.forget(1)

        assertNull(drag.draggedLocalId)
        assertNull(drag.finish())
    }

    @Test
    fun `a hovered row that leaves the screen is no longer a target`() {
        val drag = board()
        drag.start(1, Offset(10f, 25f))
        drag.move(Offset(0f, 50f))

        drag.forget(2)

        assertEquals(1L, drag.draggedLocalId)
        assertNull(drag.hoverLocalId)
        assertNull(drag.finish())
    }

    @Test
    fun `abandoning a row that is not the one carried leaves the drag alone`() {
        val drag = board()
        drag.start(1, Offset(10f, 25f))

        drag.abandon(2)

        assertEquals(1L, drag.draggedLocalId)
    }
}
