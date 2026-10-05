package ru.tinyops.turboist.core.model.view

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * Dropping one subtask onto another on the task screen makes the dropped one
 * wait for the other. The refusal has to be known while the finger is still
 * over the target, so the rule answers from the replica alone.
 */
class DependencyDropRefusalTest {
    // 1
    // ├─ 2
    // │  └─ 4
    // └─ 3
    // 6
    private val parentOf = mapOf(2L to 1L, 3L to 1L, 4L to 2L)

    private fun refusal(
        dragged: Long,
        target: Long,
        targetOpen: Boolean = true,
        edges: List<BlockEdge> = emptyList(),
    ) = dependencyDropRefusal(dragged, target, parentOf, targetOpen, edges)

    @Test
    fun `a sibling or an unrelated branch can be waited for`() {
        assertNull(refusal(2, 3))
        assertNull(refusal(4, 3))
        assertNull(refusal(4, 6))
    }

    @Test
    fun `the work above the dragged task cannot be waited for`() {
        assertEquals(DependencyDropRefusal.ANCESTOR, refusal(4, 2))
        assertEquals(DependencyDropRefusal.ANCESTOR, refusal(4, 1))
    }

    @Test
    fun `the work under the dragged task cannot be waited for`() {
        assertEquals(DependencyDropRefusal.DESCENDANT, refusal(1, 4))
    }

    @Test
    fun `finished work gives nothing to wait for`() {
        assertEquals(DependencyDropRefusal.COMPLETED, refusal(2, 3, targetOpen = false))
    }

    @Test
    fun `a wait already recorded is not recorded twice`() {
        val edges = listOf(BlockEdge(blockerLocalId = 3, blockedLocalId = 2))
        assertEquals(DependencyDropRefusal.EXISTS, refusal(2, 3, edges = edges))
    }

    @Test
    fun `a wait that closes a loop is refused, however long the loop`() {
        val direct = listOf(BlockEdge(blockerLocalId = 2, blockedLocalId = 3))
        assertEquals(DependencyDropRefusal.CYCLE, refusal(2, 3, edges = direct))
        val chain = listOf(BlockEdge(2, 6), BlockEdge(6, 3))
        assertEquals(DependencyDropRefusal.CYCLE, refusal(2, 3, edges = chain))
    }

    @Test
    fun `a malformed parent loop does not hang the rule`() {
        assertNull(dependencyDropRefusal(7, 9, mapOf(7L to 8L, 8L to 7L), true, emptyList()))
    }
}
