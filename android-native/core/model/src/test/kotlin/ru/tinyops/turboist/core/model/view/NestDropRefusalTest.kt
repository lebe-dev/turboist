package ru.tinyops.turboist.core.model.view

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Dropping one subtask onto another's middle band nests it as that row's own
 * subtask. The refusal has to be known while the finger is still over the
 * target, so the rule answers from the replica alone.
 */
class NestDropRefusalTest {
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
    ) = nestDropRefusal(dragged, target, parentOf, targetOpen)

    @Test
    fun `nesting under a sibling or an unrelated branch is fine`() {
        assertNull(refusal(2, 3))
        assertNull(refusal(4, 6))
    }

    @Test
    fun `nesting under one's own ancestor is a legitimate move up, not a cycle`() {
        assertNull(refusal(4, 1))
        assertNull(refusal(4, 2))
    }

    @Test
    fun `nesting a task under its own descendant closes a cycle`() {
        assertEquals(NestDropRefusal.DESCENDANT, refusal(1, 2))
        assertEquals(NestDropRefusal.DESCENDANT, refusal(1, 4))
        assertEquals(NestDropRefusal.DESCENDANT, refusal(2, 4))
    }

    @Test
    fun `nesting under a task that is no longer open is refused`() {
        assertEquals(NestDropRefusal.COMPLETED, refusal(2, 3, targetOpen = false))
    }

    @Test
    fun `a malformed parent loop does not hang the rule`() {
        assertNull(nestDropRefusal(7, 9, mapOf(7L to 8L, 8L to 7L), true))
    }
}

class IsNestNoopTest {
    private val parentOf = mapOf(2L to 1L, 4L to 2L)

    @Test
    fun `is a no-op only when the target is already the dragged row's parent`() {
        assertTrue(isNestNoop(2, 1, parentOf))
        assertTrue(isNestNoop(4, 2, parentOf))
        assertFalse(isNestNoop(4, 1, parentOf))
        assertFalse(isNestNoop(2, 3, parentOf))
    }
}
