package ru.tinyops.turboist.nativeapp.tasks

import androidx.compose.ui.geometry.Rect
import ru.tinyops.turboist.nativeapp.tasks.ui.islandPlacement
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class DropIslandPlacementTest {
    private val container = Rect(0f, 100f, 400f, 1000f)

    @Test
    fun `the island opens right above the carried row when there is room`() {
        val row = Rect(0f, 300f, 400f, 380f)

        val placed = islandPlacement(row, container, islandHeight = 100f, gap = 10f)

        assertEquals(190f, placed.top)
        assertTrue(placed.above)
    }

    @Test
    fun `the island drops under the row when the top of the list leaves no room`() {
        val row = Rect(0f, 150f, 400f, 230f)

        val placed = islandPlacement(row, container, islandHeight = 100f, gap = 10f)

        assertEquals(240f, placed.top)
        assertFalse(placed.above)
    }

    @Test
    fun `the island stays inside the list when neither side has room`() {
        val row = Rect(0f, 150f, 400f, 950f)

        val placed = islandPlacement(row, container, islandHeight = 100f, gap = 10f)

        assertEquals(110f, placed.top)
        assertFalse(placed.above)
    }
}
