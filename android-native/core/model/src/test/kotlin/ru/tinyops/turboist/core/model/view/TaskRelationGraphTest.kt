package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The rules that decide whether two tasks may be linked at all.
 *
 * They are the server's rules, repeated here so the refusal happens while the
 * user is still looking at the picker. A link the device accepted and the server
 * then rejected would sit in the queue and come back as a failure hours later,
 * with nothing left on screen to explain it.
 */
class TaskRelationGraphTest {
    private fun blocks(
        blocker: Long,
        blocked: Long,
    ) = BlockEdge(blockerLocalId = blocker, blockedLocalId = blocked)

    @Test
    fun `a blocking link is stored in the direction the user named it`() {
        assertEquals(
            RelationEnds(sourceLocalId = 7, targetLocalId = 1),
            relationEnds(1, 7, RelationType.BLOCKS, RelationDirection.INCOMING),
            "\"1 is blocked by 7\" stores 7 as the one that has to happen first",
        )
        assertEquals(
            RelationEnds(sourceLocalId = 1, targetLocalId = 9),
            relationEnds(1, 9, RelationType.BLOCKS, RelationDirection.OUTGOING),
        )
    }

    @Test
    fun `an informational link is stored the same way from either end`() {
        // The pair is symmetric, so the two attempts have to produce one row —
        // otherwise "a is related to b" and "b is related to a" would both be
        // stored and the task would list the same peer twice.
        val forward = relationEnds(2, 11, RelationType.RELATED, RelationDirection.OUTGOING)
        val backward = relationEnds(11, 2, RelationType.RELATED, RelationDirection.INCOMING)

        assertEquals(forward, backward)
        assertEquals(RelationEnds(sourceLocalId = 2, targetLocalId = 11), forward)
    }

    @Test
    fun `each edge reads as one of the three things a person says`() {
        assertEquals(
            TaskRelationGroup.BLOCKED_BY,
            TaskRelationGroup.of(RelationType.BLOCKS, RelationDirection.INCOMING),
        )
        assertEquals(
            TaskRelationGroup.BLOCKS,
            TaskRelationGroup.of(RelationType.BLOCKS, RelationDirection.OUTGOING),
        )
        assertEquals(
            TaskRelationGroup.RELATED,
            TaskRelationGroup.of(RelationType.RELATED, RelationDirection.INCOMING),
            "a symmetric link reads the same from both ends, so its direction is ignored",
        )
    }

    @Test
    fun `a kind this build has never heard of belongs to none of the three`() {
        assertNull(TaskRelationGroup.of(RelationType.UNKNOWN, RelationDirection.OUTGOING))
    }

    @Test
    fun `a group carries the pair of values the write is made with`() {
        assertEquals(RelationType.BLOCKS, TaskRelationGroup.BLOCKED_BY.type)
        assertEquals(RelationDirection.INCOMING, TaskRelationGroup.BLOCKED_BY.direction)
        assertEquals(RelationType.RELATED, TaskRelationGroup.RELATED.type)
    }

    @Test
    fun `a link that closes a loop of waiting is refused`() {
        // 1 waits for nothing, 2 waits for 1, 3 waits for 2. Saying 1 now waits
        // for 3 would leave all three permanently unfinishable.
        val edges = listOf(blocks(1, 2), blocks(2, 3))

        assertTrue(wouldCloseBlockingCycle(blockerLocalId = 3, blockedLocalId = 1, blockEdges = edges))
    }

    @Test
    fun `the shortest loop of all is refused`() {
        assertTrue(wouldCloseBlockingCycle(blockerLocalId = 2, blockedLocalId = 1, blockEdges = listOf(blocks(1, 2))))
        assertTrue(wouldCloseBlockingCycle(blockerLocalId = 1, blockedLocalId = 1, blockEdges = emptyList()))
    }

    @Test
    fun `a link that only joins two chains is allowed`() {
        val edges = listOf(blocks(1, 2), blocks(3, 4))

        assertFalse(wouldCloseBlockingCycle(blockerLocalId = 2, blockedLocalId = 3, blockEdges = edges))
    }

    @Test
    fun `two tasks waiting on the same third one are not a loop`() {
        // A diamond is not a cycle: 1 holds up both 2 and 3, and either may also
        // hold up 4. The walk visits 4 twice and must still terminate.
        val edges = listOf(blocks(1, 2), blocks(1, 3), blocks(2, 4), blocks(3, 4))

        assertFalse(
            wouldCloseBlockingCycle(blockerLocalId = 1, blockedLocalId = 4, blockEdges = edges),
            "1 already has to happen before 4 by two routes, and saying so again is not a loop",
        )
        assertTrue(wouldCloseBlockingCycle(blockerLocalId = 4, blockedLocalId = 1, blockEdges = edges))
    }

    @Test
    fun `a graph that already holds a loop still answers rather than spinning`() {
        // Rows are copied from a server, and a replica somehow left with a loop
        // in it must still let the screen ask this question.
        val edges = listOf(blocks(1, 2), blocks(2, 1))

        assertFalse(wouldCloseBlockingCycle(blockerLocalId = 3, blockedLocalId = 4, blockEdges = edges))
    }
}
