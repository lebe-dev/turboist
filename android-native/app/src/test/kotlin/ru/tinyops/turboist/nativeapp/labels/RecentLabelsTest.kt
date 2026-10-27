package ru.tinyops.turboist.nativeapp.labels

import org.junit.Test
import ru.tinyops.turboist.core.model.Label
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/**
 * The device's memory of the labels it applies.
 *
 * The same two rules as the recent projects: what is remembered, and what is
 * shown. The memory is deeper than the row, so a picker narrowed by a search
 * still has enough history left to fill one.
 */
class RecentLabelsTest {
    @Test
    fun `the most recent label leads and is never listed twice`() {
        val after = withRecentLabel(withRecentLabel(listOf(3L, 1L), 1), 2)

        assertEquals(listOf(2L, 1L, 3L), after)
    }

    @Test
    fun `the memory stops at its limit, dropping the least recent`() {
        val filled =
            (1L..RECENT_LABELS_MEMORY.toLong()).fold(emptyList<Long>()) { acc, id -> withRecentLabel(acc, id) }

        val after = withRecentLabel(filled, 99)

        assertEquals(RECENT_LABELS_MEMORY, after.size)
        assertEquals(99L, after.first())
        assertFalse(1L in after, "the least recent label survived the cap")
    }

    private val labels = (1L..8L).map { Label(localId = it, name = "label $it", createdAt = 0, updatedAt = 0) }

    @Test
    fun `the row leads with up to five remembered labels, most recent first`() {
        val picked = pickRecentLabels(listOf(7, 2, 5, 1, 8, 3), labels).map { it.localId }

        assertEquals(listOf(7L, 2L, 5L, 1L, 8L), picked)
    }

    @Test
    fun `a list that already fits in the row gets no row`() {
        assertEquals(emptyList(), pickRecentLabels(listOf(1, 2), labels.take(RECENT_LABELS_SHOWN)))
    }
}
