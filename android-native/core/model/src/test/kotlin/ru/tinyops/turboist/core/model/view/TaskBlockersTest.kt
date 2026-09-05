package ru.tinyops.turboist.core.model.view

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The blocker rule, asked by name.
 *
 * The counts and the names have to answer the same question — a screen that
 * refuses a completion and then lists nothing to finish first would be worse
 * than one that never refused — so every case here is asserted both ways.
 */
class TaskBlockersTest {
    private fun blocks(
        blocker: Long,
        blocked: Long,
    ) = BlockEdge(blockerLocalId = blocker, blockedLocalId = blocked)

    @Test
    fun `a task names the blockers of its own`() {
        val ids = openBlockerLocalIds(2L, listOf(blocks(1L, 2L)), emptyMap())

        assertEquals(listOf(1L), ids)
        assertEquals(mapOf(2L to 1), blockedByOpenCounts(listOf(2L), listOf(blocks(1L, 2L)), emptyMap()))
    }

    @Test
    fun `nothing blocks a task no edge points at`() {
        assertEquals(emptyList(), openBlockerLocalIds(3L, listOf(blocks(1L, 2L)), emptyMap()))
    }

    @Test
    fun `a subtask inherits what blocks the work above it`() {
        // 5 is a subtask of 2, and 1 blocks 2: finishing part of work that cannot
        // start is not progress, so 5 is blocked as well.
        val edges = listOf(blocks(1L, 2L))
        val parents = mapOf(5L to 2L)

        assertEquals(listOf(1L), openBlockerLocalIds(5L, edges, parents))
    }

    @Test
    fun `an inherited blocker inside the task's own subtree does not count`() {
        // 3 blocks its own parent 2. That is the work itself, and counting it
        // would make 2 impossible to finish by finishing its children.
        val edges = listOf(blocks(3L, 2L))
        val parents = mapOf(3L to 2L)

        assertEquals(listOf(3L), openBlockerLocalIds(2L, edges, parents))
        assertEquals(emptyList(), openBlockerLocalIds(3L, edges, parents))
    }

    @Test
    fun `a blocker named twice is named once`() {
        // The task names 1 directly and inherits it from its parent as well.
        val edges = listOf(blocks(1L, 5L), blocks(1L, 2L))
        val parents = mapOf(5L to 2L)

        assertEquals(listOf(1L), openBlockerLocalIds(5L, edges, parents))
        assertEquals(mapOf(5L to 1), blockedByOpenCounts(listOf(5L), edges, parents))
    }

    @Test
    fun `a parent chain that loops does not hang the walk`() {
        // Nothing in the product can write this, but rows are copied from a
        // server and a replica somehow left with a loop must still draw a screen.
        val parents = mapOf(1L to 2L, 2L to 1L)

        assertTrue(openBlockerLocalIds(1L, listOf(blocks(9L, 3L)), parents).isEmpty())
    }

    @Test
    fun `no edges means no blockers, whatever the tree looks like`() {
        assertEquals(emptyList(), openBlockerLocalIds(1L, emptyList(), mapOf(1L to 2L)))
    }
}
