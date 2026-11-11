package ru.tinyops.turboist.nativeapp.quickadd

import org.junit.Test
import kotlin.test.assertEquals

class ProjectActivityTest {
    @Test
    fun `the latest moment wins and an older one never moves a project back`() {
        val first = withProjectActivity(emptyMap(), projectLocalId = 1, at = 100)
        val later = withProjectActivity(first, projectLocalId = 1, at = 300)
        val stale = withProjectActivity(later, projectLocalId = 1, at = 200)

        assertEquals(mapOf(1L to 300L), later)
        assertEquals(later, stale)
    }

    @Test
    fun `the memory keeps the projects touched most recently`() {
        val full =
            (1L..3L).fold(
                emptyMap<Long, Long>(),
            ) { acc, id -> withProjectActivity(acc, id, at = id * 10, limit = 3) }
        val next = withProjectActivity(full, projectLocalId = 9, at = 100, limit = 3)

        assertEquals(setOf(2L, 3L, 9L), next.keys)
    }

    @Test
    fun `projects are ordered by their last activity, and untouched ones keep their order after the rest`() {
        val ordered = byRecentActivity(listOf(1L, 2L, 3L, 4L), mapOf(3L to 50L, 2L to 80L)) { it }

        assertEquals(listOf(2L, 3L, 1L, 4L), ordered)
    }

    @Test
    fun `the stored form round-trips and skips what it cannot read`() {
        val activity = mapOf(12L to 1_767_225_600_000L, 3L to 5L)

        assertEquals(activity, decodeProjectActivity(encodeProjectActivity(activity)))
        assertEquals(mapOf(4L to 9L), decodeProjectActivity("x:1,4:9,7:,:3,junk"))
        assertEquals(emptyMap(), decodeProjectActivity(null))
    }
}
